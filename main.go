package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Todo struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Description string             `json:"description"`
}

var mutex = &sync.Mutex{}

type wrappedWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &wrappedWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)
		log.Println(wrapped.statusCode, r.Method, r.URL.Path, time.Since(start))
	})
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("MONGO_URI not set in .env file")
	}

	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	collection := client.Database("todos").Collection("todos")

	mux.HandleFunc("POST /v1/todos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		var todo Todo
		err := json.NewDecoder(r.Body).Decode(&todo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		mutex.Lock()
		result, err := collection.InsertOne(context.TODO(), bson.M{"description": todo.Description})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mutex.Unlock()
		todo.ID = result.InsertedID.(primitive.ObjectID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(todo)
	})

	mux.HandleFunc("GET /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		idStr := r.PathValue("id")
		if idStr == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}

		id, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			http.Error(w, "id must be an integer", http.StatusBadRequest)
			return
		}

		var todo Todo
		err = collection.FindOne(context.TODO(), bson.M{"_id": id}).Decode(&todo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todo)
	})

	mux.HandleFunc("PATCH /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		id, err := primitive.ObjectIDFromHex(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		var todo Todo
		err = json.NewDecoder(r.Body).Decode(&todo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var updatedTodo Todo
		err = collection.FindOneAndUpdate(
			context.TODO(),
			bson.M{"_id": id},
			bson.M{"$set": bson.M{"description": todo.Description}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&updatedTodo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(updatedTodo)
	})

	mux.HandleFunc("DELETE /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		id, err := primitive.ObjectIDFromHex(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		result, err := collection.DeleteOne(context.TODO(), bson.M{"_id": id})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if result.DeletedCount == 0 {
			http.Error(w, "todo not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /v1/todos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")

		rows, err := collection.Find(context.TODO(), bson.M{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close(context.TODO())

		var todos []Todo
		for rows.Next(context.TODO()) {
			var todo Todo
			err := rows.Decode(&todo)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			todos = append(todos, todo)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todos)
	})

	mux.HandleFunc("OPTIONS /v1/todos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE")
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("OPTIONS /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Add("Access-Control-Allow-Methods", "GET, PATCH, DELETE")
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:    "localhost:8080",
		Handler: Logging(mux),
	}

	fmt.Println("Server running on port 8080")
	server.ListenAndServe()
}
