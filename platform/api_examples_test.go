package platform

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

func Example_openBoltDB() {
	path := filepath.Join(os.TempDir(), "goslow-open-example.db")
	defer os.Remove(path)

	db, err := OpenBoltDB(path, time.Second)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	fmt.Println(filepath.Base(db.Path()))
	// Output: goslow-open-example.db
}

func Example_saveObjects() {
	cfg := createDefaultConfiguration()
	cfg.Database.BoltDB.Enabled = true
	cfg.Database.BoltDB.FileName = filepath.Join(os.TempDir(), "goslow-save-objects-example.db")
	SetPlatformConfiguration(*cfg)
	defer DeleteDatabase(cfg.Database.BoltDB.FileName)

	if err := Database.BoltDb.SaveObjects("items", map[string]interface{}{
		"b": 2,
		"a": 1,
	}); err != nil {
		panic(err)
	}

	keys, err := Database.BoltDb.Keys("items")
	if err != nil {
		panic(err)
	}
	fmt.Println(keys)
	// Output: [a b]
}

func Example_scanPrefix() {
	cfg := createDefaultConfiguration()
	cfg.Database.BoltDB.Enabled = true
	cfg.Database.BoltDB.FileName = filepath.Join(os.TempDir(), "goslow-scan-prefix-example.db")
	SetPlatformConfiguration(*cfg)
	defer DeleteDatabase(cfg.Database.BoltDB.FileName)

	if err := Database.BoltDb.SaveObjects("events", map[string]interface{}{
		"app:1": "created",
		"app:2": "updated",
		"web:1": "created",
	}); err != nil {
		panic(err)
	}

	var appKeys []string
	if err := Database.BoltDb.ScanPrefix("events", "app:", func(key, value []byte) error {
		appKeys = append(appKeys, string(key))
		return nil
	}); err != nil {
		panic(err)
	}
	fmt.Println(appKeys)
	// Output: [app:1 app:2]
}

func Example_deleteDatabase() {
	cfg := createDefaultConfiguration()
	cfg.Database.BoltDB.Enabled = true
	cfg.Database.BoltDB.FileName = filepath.Join(os.TempDir(), "goslow-delete-example.db")
	SetPlatformConfiguration(*cfg)

	if err := SetupBoltDB(); err != nil {
		panic(err)
	}
	if err := DeleteDatabase(cfg.Database.BoltDB.FileName); err != nil {
		panic(err)
	}

	_, err := os.Stat(cfg.Database.BoltDB.FileName)
	fmt.Println(os.IsNotExist(err))
	// Output: true
}

func Example_getComponentConfiguration() {
	viper.Set("examplecomponent", map[string]interface{}{"name": "demo", "limit": 2})

	var conf struct {
		Name  string
		Limit int
	}
	if err := GetComponentConfiguration("examplecomponent", &conf); err != nil {
		panic(err)
	}

	fmt.Println(conf.Name, conf.Limit)
	// Output: demo 2
}

func Example_routeAuthStrategies() {
	strategies, err := routeAuthStrategies(Route{Auth: AuthLocalJWT})
	if err != nil {
		panic(err)
	}
	fmt.Println(strategies)

	explicit, err := routeAuthStrategies(Route{Auth: AuthLocalOrOIDC})
	if err != nil {
		panic(err)
	}
	fmt.Println(explicit)
	// Output:
	// [local-jwt]
	// [local-jwt oauth]
}

func Example_requestIDMiddleware() {
	cfg := createDefaultConfiguration()
	// Keep the example output limited to the request id.
	cfg.Log.Level = "error"
	SetPlatformConfiguration(*cfg)

	router, err := newRouter(Routes{{
		Path:   "/ping",
		Method: http.MethodGet,
		Auth:   AuthNone,
		HandlerFunc: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	}})
	if err != nil {
		panic(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/ping", nil)
	request.Header.Set(RequestIDHeader, "demo-request-id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	fmt.Println(response.Header().Get(RequestIDHeader))
	// Output: demo-request-id
}
