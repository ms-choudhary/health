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

type seedIngredient struct {
	name    string
	unit    string
	cpu     float64
	protein float64
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
		{Name: "Mohit", Avatar: "MO", TargetCalories: 2200, TargetProtein: 140},
		{Name: "Sara", Avatar: "SR", TargetCalories: 1800, TargetProtein: 100},
	}
	createdUsers := make([]queries.User, 0, len(users))
	for _, u := range users {
		row, err := q.CreateUser(ctx, u)
		if err != nil {
			log.Fatal(err)
		}
		createdUsers = append(createdUsers, row)
	}

	ingredients := []seedIngredient{
		{"Oatmeal", "g", 3.9, 0.13},
		{"Banana", "piece", 89, 1.1},
		{"Grilled Chicken", "g", 1.65, 0.31},
		{"Brown Rice", "g", 1.3, 0.026},
		{"Almonds", "g", 5.8, 0.21},
		{"Greek Yogurt", "ml", 0.67, 0.10},
		{"Olive Oil", "ml", 8.8, 0.0},
		{"Egg", "piece", 78, 6.3},
		{"Avocado", "g", 1.6, 0.02},
		{"Salmon", "g", 2.08, 0.20},
	}
	createdIngredients := make([]queries.Ingredient, 0, len(ingredients))
	for _, f := range ingredients {
		row, err := q.CreateIngredient(ctx, queries.CreateIngredientParams{
			Name: f.name, Unit: f.unit, CaloriesPerUnit: f.cpu, ProteinPerUnit: f.protein,
		})
		if err != nil {
			log.Fatal(err)
		}
		createdIngredients = append(createdIngredients, row)
	}

	ingredientByName := make(map[string]queries.Ingredient, len(createdIngredients))
	for _, f := range createdIngredients {
		ingredientByName[f.Name] = f
	}
	type seedRecipeIngredient struct {
		ingredientName string
		quantity float64
	}
	type seedRecipe struct {
		name        string
		ingredients []seedRecipeIngredient
		tags        []string
	}
	demoRecipes := []seedRecipe{
		{
			name: "Yogurt Parfait",
			ingredients: []seedRecipeIngredient{
				{"Greek Yogurt", 200},
				{"Almonds", 15},
				{"Banana", 1},
			},
			tags: []string{"Breakfast", "Vegetarian"},
		},
		{
			name: "Avocado Egg Bowl",
			ingredients: []seedRecipeIngredient{
				{"Avocado", 100},
				{"Egg", 2},
				{"Olive Oil", 5},
			},
			tags: []string{"Breakfast", "High Protein"},
		},
		{
			name: "Chicken Rice Plate",
			ingredients: []seedRecipeIngredient{
				{"Grilled Chicken", 150},
				{"Brown Rice", 180},
				{"Olive Oil", 5},
			},
			tags: []string{"Lunch", "High Protein"},
		},
		{
			name: "Oatmeal Breakfast",
			ingredients: []seedRecipeIngredient{
				{"Oatmeal", 80},
				{"Banana", 1},
				{"Almonds", 10},
			},
			tags: []string{"Breakfast", "Vegetarian"},
		},
	}

	tagByName := make(map[string]queries.FoodTag)
	ensureTag := func(name string) queries.FoodTag {
		if t, ok := tagByName[name]; ok {
			return t
		}
		t, err := q.CreateFoodTag(ctx, name)
		if err != nil {
			log.Fatal(err)
		}
		tagByName[name] = t
		return t
	}
	createdRecipes := make([]queries.Recipe, 0, len(demoRecipes))
	for _, dr := range demoRecipes {
		recipe, err := q.CreateRecipe(ctx, dr.name)
		if err != nil {
			log.Fatal(err)
		}
		for _, ing := range dr.ingredients {
			ingredient, ok := ingredientByName[ing.ingredientName]
			if !ok {
				log.Fatalf("seed recipe ingredient %q not found", ing.ingredientName)
			}
			if _, err := q.AddRecipeIngredient(ctx, queries.AddRecipeIngredientParams{
				RecipeID: recipe.ID,
				IngredientID:   ingredient.ID,
				Quantity: ing.quantity,
			}); err != nil {
				log.Fatal(err)
			}
		}
		for _, tagName := range dr.tags {
			tag := ensureTag(tagName)
			if err := q.AddRecipeFoodTag(ctx, queries.AddRecipeFoodTagParams{
				RecipeID:  recipe.ID,
				FoodTagID: tag.ID,
			}); err != nil {
				log.Fatal(err)
			}
		}
		createdRecipes = append(createdRecipes, recipe)
	}

	rng := rand.New(rand.NewSource(42))
	today := time.Now().UTC()
	for _, u := range createdUsers {
		baseWeight := 70.0 + rng.Float64()*15
		for d := 13; d >= 0; d-- {
			date := today.AddDate(0, 0, -d).Format("2006-01-02")
			weight := baseWeight + rng.Float64()*1.5 - float64(d)*0.05
			steps := int64(4000 + rng.Intn(8000))
			if _, err := q.UpsertMetrics(ctx, queries.UpsertMetricsParams{
				UserID: u.ID, Date: date,
				Weight: &weight, Steps: &steps,
			}); err != nil {
				log.Fatal(err)
			}
			mealsPerDay := 2 + rng.Intn(2)
			for i := 0; i < mealsPerDay; i++ {
				recipe := createdRecipes[rng.Intn(len(createdRecipes))]
				ings, err := q.GetRecipeIngredients(ctx, recipe.ID)
				if err != nil {
					log.Fatal(err)
				}
				if len(ings) == 0 {
					continue
				}
				servings := 1.0 + float64(rng.Intn(2))
				for _, ing := range ings {
					qty := ing.Quantity * servings
					rid, rname := recipe.ID, recipe.Name
					if _, err := q.AddLogEntry(ctx, queries.AddLogEntryParams{
						UserID: u.ID, IngredientID: &ing.IngredientID, Date: date,
						IngredientName: ing.IngredientName, IngredientUnit: ing.IngredientUnit,
						CaloriesPerUnit: ing.CaloriesPerUnit, ProteinPerUnit: ing.ProteinPerUnit,
						Quantity: qty,
						Calories: ing.CaloriesPerUnit * qty, Protein: ing.ProteinPerUnit * qty,
						SourceRecipeID: &rid, SourceRecipeName: &rname, SourceRecipeServings: &servings,
					}); err != nil {
						log.Fatal(err)
					}
				}
			}
		}
	}

	fmt.Printf("Seeded %d users, %d ingredients, %d recipes, %d food tags, ~14 days of log entries each.\n",
		len(createdUsers), len(createdIngredients), len(createdRecipes), len(tagByName))
}
