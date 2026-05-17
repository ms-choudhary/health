package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"health/db"
	"health/db/queries"
)

type seedFood struct {
	name string
	unit string
	cpu  float64
}

func main() {
	dbPath := os.Getenv("HEALTH_DB")
	if dbPath == "" {
		dbPath = "health.db"
	}

	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	ctx := context.Background()
	q := database.Queries

	users := []queries.CreateUserParams{
		{Name: "Mohit", Avatar: "MO"},
		{Name: "Sara", Avatar: "SR"},
	}
	createdUsers := make([]queries.User, 0, len(users))
	for _, u := range users {
		row, err := q.CreateUser(ctx, u)
		if err != nil {
			log.Fatal(err)
		}
		createdUsers = append(createdUsers, row)
	}

	foods := []seedFood{
		{"Oatmeal", "g", 3.9},
		{"Banana", "piece", 89},
		{"Grilled Chicken", "g", 1.65},
		{"Brown Rice", "g", 1.3},
		{"Almonds", "g", 5.8},
		{"Greek Yogurt", "ml", 0.67},
		{"Olive Oil", "ml", 8.8},
		{"Egg", "piece", 78},
		{"Avocado", "g", 1.6},
		{"Salmon", "g", 2.08},
	}
	createdFoods := make([]queries.Food, 0, len(foods))
	for _, f := range foods {
		row, err := q.CreateFood(ctx, queries.CreateFoodParams{
			Name: f.name, Unit: f.unit, CaloriesPerUnit: f.cpu,
		})
		if err != nil {
			log.Fatal(err)
		}
		createdFoods = append(createdFoods, row)
	}

	rng := rand.New(rand.NewSource(42))
	today := time.Now().UTC()
	for _, u := range createdUsers {
		baseWeight := 70.0 + rng.Float64()*15
		for d := 13; d >= 0; d-- {
			date := today.AddDate(0, 0, -d).Format("2006-01-02")
			weight := baseWeight + rng.Float64()*1.5 - float64(d)*0.05
			steps := int64(4000 + rng.Intn(8000))
			target := int64(2000)
			if _, err := q.UpsertMetrics(ctx, queries.UpsertMetricsParams{
				UserID: u.ID, Date: date,
				Weight: &weight, Steps: &steps, TargetCalories: &target,
			}); err != nil {
				log.Fatal(err)
			}
			entriesPerDay := 3 + rng.Intn(3)
			for i := 0; i < entriesPerDay; i++ {
				f := createdFoods[rng.Intn(len(createdFoods))]
				qty := 50 + rng.Float64()*200
				if f.Unit == "piece" {
					qty = float64(1 + rng.Intn(2))
				}
				if _, err := q.AddLogEntry(ctx, queries.AddLogEntryParams{
					UserID: u.ID, FoodID: &f.ID, Date: date,
					FoodName: f.Name, FoodUnit: f.Unit,
					CaloriesPerUnit: f.CaloriesPerUnit,
					Quantity:        qty,
					Calories:        f.CaloriesPerUnit * qty,
				}); err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	fmt.Printf("Seeded %d users, %d foods, ~14 days of log entries each.\n",
		len(createdUsers), len(createdFoods))
}
