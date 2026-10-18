package main

import (
	"fmt"
	"log"

	"github.com/Mallekoppie/goslow/platform"
	"go.uber.org/zap"
)

// Example data structures
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Age      int    `json:"age"`
}

type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

func main() {
	// Initialize the platform configuration
	config := platform.Config{}
	config.Component.ComponentName = "BoltDB Example"
	config.Log.Level = "debug"
	config.Log.FileLoggingEnabled = false
	config.Database.BoltDB.Enabled = true
	config.Database.BoltDB.FileName = "./example_database.db"

	// Configure the platform
	platform.SetPlatformConfiguration(config)

	// Setup BoltDB
	err := platform.SetupBoltDB()
	if err != nil {
		log.Fatal("Failed to setup BoltDB:", err)
	}
	defer platform.Database.BoltDb.Close()

	fmt.Println("=== BoltDB Example Application ===")

	// 1. Save objects
	fmt.Println("1. Saving objects to database...")
	saveExamples()

	// 2. Read single objects
	fmt.Println("\n2. Reading individual objects...")
	readExamples()

	// 3. Read all objects from a bucket
	fmt.Println("\n3. Reading all objects from buckets...")
	readAllExamples()

	// 4. Remove a single object
	fmt.Println("\n4. Removing a single object...")
	removeObjectExample()

	// 5. Remove an entire bucket
	fmt.Println("\n5. Removing an entire bucket...")
	removeBucketExample()

	// 6. Cleanup - Remove DB file
	fmt.Println("\n6. Cleanup - Removing database file...")
	cleanupExample()

	fmt.Println("\n=== Example completed successfully ===")
}

func saveExamples() {
	// Save users
	users := []User{
		{ID: "user1", Username: "alice", Email: "alice@example.com", Age: 30},
		{ID: "user2", Username: "bob", Email: "bob@example.com", Age: 25},
		{ID: "user3", Username: "charlie", Email: "charlie@example.com", Age: 35},
	}

	for _, user := range users {
		err := platform.Database.BoltDb.SaveObject("users", user.ID, user)
		if err != nil {
			log.Printf("Error saving user %s: %v", user.ID, err)
		} else {
			fmt.Printf("  ✓ Saved user: %s (%s)\n", user.Username, user.ID)
		}
	}

	// Save products
	products := []Product{
		{ID: "prod1", Name: "Laptop", Price: 999.99},
		{ID: "prod2", Name: "Mouse", Price: 29.99},
		{ID: "prod3", Name: "Keyboard", Price: 79.99},
	}

	for _, product := range products {
		err := platform.Database.BoltDb.SaveObject("products", product.ID, product)
		if err != nil {
			log.Printf("Error saving product %s: %v", product.ID, err)
		} else {
			fmt.Printf("  ✓ Saved product: %s (%s)\n", product.Name, product.ID)
		}
	}
}

func readExamples() {
	// Read a user
	var user User
	err := platform.Database.BoltDb.ReadObject("users", "user1", &user)
	if err != nil {
		log.Printf("Error reading user: %v", err)
	} else {
		fmt.Printf("  ✓ Read user: %s (Email: %s, Age: %d)\n", user.Username, user.Email, user.Age)
		platform.Log.Debug("User object retrieved", zap.Any("user", user))
	}

	// Read a product
	var product Product
	err = platform.Database.BoltDb.ReadObject("products", "prod2", &product)
	if err != nil {
		log.Printf("Error reading product: %v", err)
	} else {
		fmt.Printf("  ✓ Read product: %s (Price: $%.2f)\n", product.Name, product.Price)
		platform.Log.Debug("Product object retrieved", zap.Any("product", product))
	}
}

func readAllExamples() {
	// Read all users
	allUsers, err := platform.Database.BoltDb.ReadAllObjects("users")
	if err != nil {
		log.Printf("Error reading all users: %v", err)
	} else {
		fmt.Printf("  ✓ Found %d users in database:\n", len(allUsers))
		for key, value := range allUsers {
			fmt.Printf("    - Key: %s, Value: %s\n", key, value)
		}
	}

	// Read all products
	allProducts, err := platform.Database.BoltDb.ReadAllObjects("products")
	if err != nil {
		log.Printf("Error reading all products: %v", err)
	} else {
		fmt.Printf("  ✓ Found %d products in database:\n", len(allProducts))
		for key, value := range allProducts {
			fmt.Printf("    - Key: %s, Value: %s\n", key, value)
		}
	}
}

func removeObjectExample() {
	// Remove user2
	err := platform.Database.BoltDb.RemoveObject("users", "user2")
	if err != nil {
		log.Printf("Error removing object: %v", err)
	} else {
		fmt.Println("  ✓ Removed user2 from users bucket")
	}

	// Verify it's removed
	var user User
	err = platform.Database.BoltDb.ReadObject("users", "user2", &user)
	if err != nil {
		fmt.Printf("  ✓ Confirmed: user2 no longer exists (error: %v)\n", err)
	}

	// Show remaining users
	allUsers, _ := platform.Database.BoltDb.ReadAllObjects("users")
	fmt.Printf("  ✓ Remaining users: %d\n", len(allUsers))
}

func removeBucketExample() {
	// Remove the entire products bucket
	err := platform.Database.BoltDb.RemoveBucket("products")
	if err != nil {
		log.Printf("Error removing bucket: %v", err)
	} else {
		fmt.Println("  ✓ Removed entire 'products' bucket")
	}

	// Try to read from the removed bucket
	allProducts, err := platform.Database.BoltDb.ReadAllObjects("products")
	if err != nil {
		fmt.Printf("  ✓ Confirmed: products bucket no longer exists (error: %v)\n", err)
	} else if len(allProducts) == 0 {
		fmt.Println("  ✓ Confirmed: products bucket is empty")
	}
}

func cleanupExample() {
	// Remove the database file
	err := platform.Database.BoltDb.RemoveDBFile()
	if err != nil {
		log.Printf("Error removing database file: %v", err)
	} else {
		fmt.Println("  ✓ Database file removed successfully")
	}
}
