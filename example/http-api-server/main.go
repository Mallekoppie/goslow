package main

import (
	"log"

	"github.com/Mallekoppie/goslow/platform"
)

func main() {
	conf, err := platform.GetPlatformConfiguration()
	if err != nil {
		log.Fatalf("unable to load platform configuration: %v", err)
	}

	if conf.Database.BoltDB.Enabled {
		if err := platform.SetupBoltDB(); err != nil {
			log.Fatalf("unable to open the bolt database: %v", err)
		}
		defer func() {
			if err := platform.Database.BoltDb.Close(); err != nil {
				log.Printf("unable to close the bolt database: %v", err)
			}
		}()
	}

	if err := platform.StartHttpServer(Routes); err != nil {
		log.Fatalf("http server stopped: %v", err)
	}
}
