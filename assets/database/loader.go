package database

import (
	_ "embed"

	"github.com/wowsims/classic/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

//go:embed db.bin
var dbBytes []byte

//go:embed forever.bin
var foreverBytes []byte

func Load() *proto.UIDatabase {
	return load(dbBytes)
}

// LoadForever returns the client-derived item catalog used only by Forever players and tools.
func LoadForever() *proto.UIDatabase {
	return load(foreverBytes)
}

func load(raw []byte) *proto.UIDatabase {
	db := &proto.UIDatabase{}
	if err := googleProto.Unmarshal(raw, db); err != nil {
		panic(err)
	}
	return db
}
