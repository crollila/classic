// Pack an already-written assets/database/db.json into db.bin.
//
// gen_db regenerates the database from its scraped inputs and would discard
// anything added afterwards. This tool does the opposite: it takes db.json as
// authoritative and rewrites the binary the engine embeds, so a reviewed
// Forever-provisional item added to db.json becomes simulatable without
// re-scraping. It makes no network requests and edits no item.
//
//	go run ./tools/database/pack_db -dbDir=assets/database
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/wowsims/classic/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

var dbDir = flag.String("dbDir", "assets/database", "Directory holding db.json and db.bin")
var name = flag.String("name", "db", "Database base name: db or leftover_db")
var jsonPathFlag = flag.String("jsonPath", "", "Explicit input JSON path (overrides dbDir/name)")
var binPathFlag = flag.String("binPath", "", "Explicit output binary path (overrides dbDir/name)")

func main() {
	flag.Parse()
	jsonPath := *jsonPathFlag
	if jsonPath == "" {
		jsonPath = fmt.Sprintf("%s/%s.json", *dbDir, *name)
	}
	binPath := *binPathFlag
	if binPath == "" {
		binPath = fmt.Sprintf("%s/%s.bin", *dbDir, *name)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		log.Fatalf("[ERROR] Failed to read %s: %s", jsonPath, err.Error())
	}

	// Unknown fields stay rejected here, exactly as the browser rejects them.
	// A malformed addition fails at pack time instead of breaking the site.
	db := &proto.UIDatabase{}
	if err := protojson.Unmarshal(raw, db); err != nil {
		log.Fatalf("[ERROR] %s is not a valid UIDatabase: %s", jsonPath, err.Error())
	}

	packed, err := googleProto.Marshal(db)
	if err != nil {
		log.Fatalf("[ERROR] Failed to marshal %s: %s", binPath, err.Error())
	}
	if err := os.WriteFile(binPath, packed, 0666); err != nil {
		log.Fatalf("[ERROR] Failed to write %s: %s", binPath, err.Error())
	}
	fmt.Printf("Packed %d items, %d enchants from %s into %s\n", len(db.Items), len(db.Enchants), jsonPath, binPath)
}
