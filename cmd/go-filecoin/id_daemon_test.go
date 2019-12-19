package commands_test

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/go-filecoin/internal/app/go-filecoin/node/test"
	th "github.com/filecoin-project/go-filecoin/internal/pkg/testhelpers"
	tf "github.com/filecoin-project/go-filecoin/internal/pkg/testhelpers/testflags"
)

func TestId(t *testing.T) {
	tf.IntegrationTest(t)

	ctx := context.Background()

	builder := test.NewNodeBuilder(t)
	n := builder.BuildAndStart(ctx)
	defer n.Stop(ctx)
	cmdClient, done := test.RunNodeAPI(ctx, n, t)
	defer done()

	id := cmdClient.RunSuccess(ctx, "id")

	idContent := id.ReadStdout()
	assert.Containsf(t, idContent, "/ip4/127.0.0.1/tcp/", "default addr")
	assert.Contains(t, idContent, "ID")
}

func TestIdFormat(t *testing.T) {
	tf.IntegrationTest(t)

	ctx := context.Background()

	builder := test.NewNodeBuilder(t)
	n := builder.BuildAndStart(ctx)
	defer n.Stop(ctx)
	cmdClient, done := test.RunNodeAPI(ctx, n, t)
	defer done()

	idContent := cmdClient.RunSuccess(
		ctx,
		"id",
		"--format=\"<id>\\t<aver>\\t<pver>\\t<pubkey>\\n<addrs>\"",
	).ReadStdout()

	assert.Contains(t, idContent, "\t")
	assert.Contains(t, idContent, "\n")
	assert.Containsf(t, idContent, "/ip4/127.0.0.1/tcp/", "default addr")
	assert.NotContains(t, idContent, "ID")
}

func TestPersistId(t *testing.T) {
	tf.IntegrationTest(t)

	// we need to control this
	dir, err := ioutil.TempDir("", "go-fil-test")
	require.NoError(t, err)

	// Start a demon in dir
	d1 := th.NewDaemon(t, th.ContainerDir(dir)).Start()

	// get the id and kill it
	id1 := d1.GetID()
	d1.Stop()

	// restart the daemon
	d2 := th.NewDaemon(t, th.ShouldInit(false), th.ContainerDir(dir)).Start()

	// get the id and compare to previous
	id2 := d2.GetID()
	d2.ShutdownSuccess()
	t.Logf("d1: %s", d1.ReadStdout())
	t.Logf("d2: %s", d2.ReadStdout())
	assert.Equal(t, id1, id2)
}