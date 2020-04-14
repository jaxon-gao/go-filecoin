package test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	commands "github.com/filecoin-project/go-filecoin/cmd/go-filecoin"
	cmdkit "github.com/ipfs/go-ipfs-cmdkit"
	cmds "github.com/ipfs/go-ipfs-cmds"

	"github.com/filecoin-project/go-filecoin/build/project"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-filecoin/internal/app/go-filecoin/node"
	"github.com/filecoin-project/go-filecoin/internal/pkg/clock"
	"github.com/filecoin-project/go-filecoin/internal/pkg/constants"
	gengen "github.com/filecoin-project/go-filecoin/tools/gengen/util"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/stretchr/testify/require"
)

// MustCreateNodesWithBootstrap creates an in-process test setup capable of testing communication between nodes.
// Every setup will have one bootstrap node (the first node that is called) that is setup to have power to mine.
// All of the proofs for the set-up are fake (but additional nodes will still need to create miners and add storage to
// gain power). All nodes will be started and connected to each other. The returned cancel function ensures all nodes
// are stopped when the test is over.
func MustCreateNodesWithBootstrap(ctx context.Context, t *testing.T, additionalNodes uint) ([]*node.Node, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	nodes := make([]*node.Node, 1+additionalNodes)

	// set up paths and fake clock.
	presealPath := project.Root("fixtures/genesis-sectors")
	genCfgPath := project.Root("fixtures/setup.json")
	genTime := int64(1000000000)
	blockTime := 30 * time.Second
	fakeClock := clock.NewFake(time.Unix(genTime, 0))

	// Load genesis config fixture.
	genCfg := loadGenesisConfig(t, genCfgPath)
	genCfg.Miners = append(genCfg.Miners, &gengen.CreateStorageMinerConfig{
		Owner:      5,
		SectorSize: constants.DevSectorSize,
	})
	seed := node.MakeChainSeed(t, genCfg)
	chainClock := clock.NewChainClockFromClock(uint64(genTime), blockTime, fakeClock)

	// create bootstrap miner
	bootstrapMiner := NewNodeBuilder(t).
		WithGenesisInit(seed.GenesisInitFunc).
		WithBuilderOpt(node.FakeProofVerifierBuilderOpts()...).
		WithBuilderOpt(node.ChainClockConfigOption(chainClock)).
		Build(ctx)

	_, _, err := initNodeGenesisMiner(t, bootstrapMiner, seed, genCfg.Miners[0].Owner, presealPath, genCfg.Miners[0].SectorSize)
	require.NoError(t, err)
	err = bootstrapMiner.Start(ctx)
	require.NoError(t, err)

	nodes[0] = bootstrapMiner

	// create additional nodes
	for i := uint(0); i < additionalNodes; i++ {
		node := NewNodeBuilder(t).
			WithGenesisInit(seed.GenesisInitFunc).
			WithConfig(node.DefaultAddressConfigOpt(seed.Addr(t, int(i+1)))).
			WithBuilderOpt(node.FakeProofVerifierBuilderOpts()...).
			WithBuilderOpt(node.ChainClockConfigOption(chainClock)).
			Build(ctx)
		seed.GiveKey(t, node, int(i+1))
		err = node.Start(ctx)
		require.NoError(t, err)
		nodes[i+1] = node
	}

	// connect all nodes
	for i := 0; i < len(nodes); i++ {
		for j := 0; j < i; j++ {
			node.ConnectNodes(t, nodes[i], nodes[j])
		}
	}

	// start simulated mining and wait for shutdown
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				fakeClock.Advance(blockTime)
				_, err := bootstrapMiner.BlockMining.BlockMiningAPI.MiningOnce(ctx)
				require.NoError(t, err)
			}
		}
	}()

	return nodes, cancel
}

func RunCommandInProcess(ctx context.Context, nd *node.Node, cmd *cmds.Command, optMap cmdkit.OptMap, args ...string) (interface{}, error) {
	req, err := cmds.NewRequest(ctx, []string{}, optMap, args, nil, cmd)
	if err != nil {
		return nil, err
	}
	emitter := &testEmitter{}
	err = cmd.Run(req, emitter, commands.CreateServerEnv(ctx, nd))
	if err != nil {
		return nil, err
	}
	if emitter.err != nil {
		return nil, err
	}
	return emitter.value, nil
}

func initNodeGenesisMiner(t *testing.T, nd *node.Node, seed *node.ChainSeed, minerIdx int, presealPath string, sectorSize abi.SectorSize) (address.Address, address.Address, error) {
	seed.GiveKey(t, nd, minerIdx)
	miner, owner := seed.GiveMiner(t, nd, 0)

	err := node.ImportPresealedSectors(nd.Repo, presealPath, sectorSize, true)
	require.NoError(t, err)
	return miner, owner, err
}

func loadGenesisConfig(t *testing.T, path string) *gengen.GenesisCfg {
	configFile, err := os.Open(path)
	if err != nil {
		t.Errorf("failed to open config file %s: %s", path, err)
	}
	defer func() { _ = configFile.Close() }()

	var cfg gengen.GenesisCfg
	if err := json.NewDecoder(configFile).Decode(&cfg); err != nil {
		t.Errorf("failed to parse config: %s", err)
	}
	return &cfg
}

type testEmitter struct {
	value interface{}
	err   error
}

func (t *testEmitter) SetLength(_ uint64) {}
func (t *testEmitter) Close() error       { return nil }

func (t *testEmitter) CloseWithError(err error) error {
	t.err = err
	return nil
}

func (t *testEmitter) Emit(value interface{}) error {
	t.value = value
	return nil
}
