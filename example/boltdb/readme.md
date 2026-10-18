# BoltDB Example

This example demonstrates all BoltDB functions available in the goslow platform.

## Overview

This example shows how to:
1. Configure and setup BoltDB
2. Save objects to the database
3. Read individual objects
4. Read all objects from a bucket
5. Remove individual objects
6. Remove entire buckets
7. Clean up database files

## Running the Example

### Using Configuration in Code

```bash
cd example/boltdb
go run main.go
```

The example uses programmatic configuration (no config.yml file required), demonstrating the new pattern where platform configuration is done in code.

### Using Configuration File (Alternative)

If you prefer to use a config file, the example includes a `config.yml` file. To use it, you would need to call:

```go
platform.InitializePlatformConfiguration()
```

instead of:

```go
platform.SetPlatformConfiguration(config)
```

## What the Example Demonstrates

### 1. Platform Configuration
```go
config := platform.Config{}
config.Database.BoltDB.Enabled = true
config.Database.BoltDB.FileName = "./example_database.db"
platform.SetPlatformConfiguration(config)
```

### 2. BoltDB Setup
```go
err := platform.SetupBoltDB()
if err != nil {
    log.Fatal("Failed to setup BoltDB:", err)
}
defer platform.Database.BoltDb.Close()
```

### 3. Save Objects
```go
user := User{ID: "user1", Username: "alice", Email: "alice@example.com"}
err := platform.Database.BoltDb.SaveObject("users", user.ID, user)
```

### 4. Read Objects
```go
var user User
err := platform.Database.BoltDb.ReadObject("users", "user1", &user)
```

### 5. Read All Objects
```go
allUsers, err := platform.Database.BoltDb.ReadAllObjects("users")
// Returns map[string]string where values are JSON strings
```

### 6. Remove Objects
```go
err := platform.Database.BoltDb.RemoveObject("users", "user1")
```

### 7. Remove Bucket
```go
err := platform.Database.BoltDb.RemoveBucket("users")
```

### 8. Remove Database File
```go
err := platform.DeleteDatabase(config.Database.BoltDB.FileName)
```

## Data Structures

The example uses two sample data structures:

**User:**
```go
type User struct {
    ID       string
    Username string
    Email    string
    Age      int
}
```

**Product:**
```go
type Product struct {
    ID    string
    Name  string
    Price float64
}
```

## Expected Output

When you run the example, you should see output similar to:

```
=== BoltDB Example Application ===

1. Saving objects to database...
  ✓ Saved user: alice (user1)
  ✓ Saved user: bob (user2)
  ✓ Saved user: charlie (user3)
  ✓ Saved product: Laptop (prod1)
  ✓ Saved product: Mouse (prod2)
  ✓ Saved product: Keyboard (prod3)

2. Reading individual objects...
  ✓ Read user: alice (Email: alice@example.com, Age: 30)
  ✓ Read product: Mouse (Price: $29.99)

3. Reading all objects from buckets...
  ✓ Found 3 users in database:
    - Key: user1, Value: {"id":"user1","username":"alice",...}
    - Key: user2, Value: {"id":"user2","username":"bob",...}
    - Key: user3, Value: {"id":"user3","username":"charlie",...}
  ✓ Found 3 products in database:
    - Key: prod1, Value: {"id":"prod1","name":"Laptop",...}
    - Key: prod2, Value: {"id":"prod2","name":"Mouse",...}
    - Key: prod3, Value: {"id":"prod3","name":"Keyboard",...}

4. Removing a single object...
  ✓ Removed user2 from users bucket
  ✓ Confirmed: user2 no longer exists
  ✓ Remaining users: 2

5. Removing an entire bucket...
  ✓ Removed entire 'products' bucket
  ✓ Confirmed: products bucket no longer exists

6. Cleanup - Removing database file...
  ✓ Database file removed successfully

=== Example completed successfully ===
```

## Notes

- The example automatically cleans up after itself by removing the database file
- All BoltDB operations are logged at the debug level
- Objects are automatically marshalled to/from JSON
- The database file will be created in the current directory as `example_database.db`
