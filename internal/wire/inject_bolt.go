package wire

import (
	"github.com/contentforward/bolt-ui/adapters"
	"github.com/contentforward/bolt-ui/internal/config"
	"github.com/google/wire"
	bolt "go.etcd.io/bbolt"
)

//lint:ignore U1000 because
var boltSet = wire.NewSet(
	newBolt,
)

func newBolt(conf *config.Config) (*bolt.DB, error) {
	return adapters.NewBolt(conf.DatabaseFile)
}
