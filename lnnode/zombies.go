package lnnode

import (
	"github.com/coreos/bbolt"
	"github.com/lightningnetwork/lnd/channeldb"
)

const (
	maxZombies = 10000
)

var (
	edgeBucket   = []byte("graph-edge")
	zombieBucket = []byte("zombie-index")
)

func deleteZombies(chanDB *channeldb.DB) error {
	err := chanDB.Update(func(tx *bbolt.Tx) error {
		edges := tx.Bucket(edgeBucket)
		if edges == nil {
			return channeldb.ErrGraphNoEdgesFound
		}
		zombies := edges.Bucket(zombieBucket)
		if zombies == nil {
			return nil
		}
		if zombies.Stats().KeyN > maxZombies {
			return edges.DeleteBucket(zombieBucket)
		}
		return nil
	})
	return err
}