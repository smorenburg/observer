package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

var port string
var client *mongo.Client
var svc = "observer"

var (
	histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: "http_server",
		Name:      "resp_time",
		Help:      "Request response time",
	}, []string{
		"service",
		"code",
		"method",
		"path",
	})
)

type Person struct {
	ID        primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	FirstName string             `json:"FirstName,omitempty" bson:"FirstName,omitempty"`
	LastName  string             `json:"LastName,omitempty" bson:"LastName,omitempty"`
}

func init() {
	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting the application on port %s...\n", port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	host := os.Getenv("DB")
	if host == "" {
		host = "localhost"
	}
	uri := "mongodb://" + host + ":27017"
	clientOptions := options.Client().ApplyURI(uri)
	client, _ = mongo.Connect(ctx, clientOptions)
	log.Printf("Connecting to the database on %s...\n", host)
	err := client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatalf("error: %s", err)
	}
	log.Printf("Successfully connected to the database")
	prometheus.MustRegister(histogram)
}

func main() {
	h := mux.NewRouter()
	h.HandleFunc("/", indexEndpoint).Methods("GET")
	h.HandleFunc("/health", healthEndpoint).Methods("GET")
	h.HandleFunc("/person", createPersonEndpoint).Methods("POST")
	h.HandleFunc("/people", getPeopleEndpoint).Methods("GET")
	h.HandleFunc("/person/{id}", getPersonEndpoint).Methods("GET")

	hm := mux.NewRouter()
	hm.Handle("/metrics", metricsEndpoint()).Methods("GET")

	go func() {
		log.Fatal(http.ListenAndServe(":9090", hm))
	}()
	log.Fatal(http.ListenAndServe(":"+port, h))
}

func recordMetrics(start time.Time, req *http.Request, code int) {
	duration := time.Since(start)
	histogram.With(
		prometheus.Labels{
			"service": svc,
			"code":    fmt.Sprintf("%d", code),
			"method":  req.Method,
			"path":    req.URL.Path,
		},
	).Observe(duration.Seconds())
}

func latency(l string) {
	var ms int
	if l == "random" {
		rand.Seed(time.Now().UnixNano())
		ms = rand.Intn(1000)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	} else {
		var err error
		if ms, err = strconv.Atoi(l); err == nil {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
	log.Printf("Request delayed for %v ms\n", ms)
}

func httpError(e string) (int, string) {
	var c int
	var m string
	if e == "random" {
		// Generate random HTTP server error 500.
		rand.Seed(time.Now().UnixNano())
		n := rand.Intn(10)
		if n == 0 {
			c = http.StatusInternalServerError
			m = "500 Internal Server Error"
		} else {
			c = http.StatusOK
		}
	} else {
		// Generate specified HTTP error (if exists).
		switch e {
		case "400":
			c = http.StatusBadRequest
			m = "400 Bad Request"
		case "401":
			c = http.StatusUnauthorized
			m = "401 Unauthorized"
		case "500":
			c = http.StatusInternalServerError
			m = "500 Internal Server Error"
		default:
			c = http.StatusOK
		}
	}
	return c, m
}

func metricsEndpoint() http.Handler {
	return promhttp.Handler()
}

func indexEndpoint(w http.ResponseWriter, r *http.Request) {
	c := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, r, c) }()
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)

	// Add artificial latency.
	l := r.URL.Query().Get("latency")
	if len(l) > 0 {
		latency(l)
	}
	// Generate HTTP errors.
	e := r.URL.Query().Get("error")
	if len(e) > 0 {
		var resp string
		c, resp = httpError(e)
		if c != 200 {
			log.Printf("error: " + resp)
			http.Error(w, resp, c)
			return
		}
	}

	resp := "Hello, world!\n"
	_, _ = io.WriteString(w, resp)
}

func healthEndpoint(w http.ResponseWriter, r *http.Request) {
	_, _ = io.WriteString(w, "200 OK\n")
}

func createPersonEndpoint(w http.ResponseWriter, r *http.Request) {
	c := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, r, c) }()
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)
	w.Header().Set("Content-Type", "application/json")

	var person Person
	err := json.NewDecoder(r.Body).Decode(&person)
	if err != nil {
		log.Fatalf("error: %s", err)
	}

	coll := client.Database("observer").Collection("people")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := coll.InsertOne(ctx, person)
	if err != nil {
		log.Fatalf("error: %s", err)
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func getPersonEndpoint(w http.ResponseWriter, r *http.Request) {
	c := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, r, c) }()
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)
	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		log.Fatalf("error: %s", err)
	}

	var person Person
	coll := client.Database("observer").Collection("people")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = coll.FindOne(ctx, Person{ID: id}).Decode(&person)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}

	_ = json.NewEncoder(w).Encode(person)
}

func getPeopleEndpoint(w http.ResponseWriter, r *http.Request) {
	c := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, r, c) }()
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)
	w.Header().Set("Content-Type", "application/json")

	var people []Person
	coll := client.Database("observer").Collection("people")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, err = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		if err != nil {
			log.Fatalf("error: %s", err)
		}
		return
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err = cursor.Close(ctx)
		if err != nil {
			log.Fatalf("error: %s", err)
		}
	}(cursor, ctx)
	for cursor.Next(ctx) {
		var person Person
		_ = cursor.Decode(&person)
		people = append(people, person)
	}
	if err := cursor.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}

	_ = json.NewEncoder(w).Encode(people)
}
