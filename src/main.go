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

type handler struct {
	Mux    *mux.Router
	Client *mongo.Client
}

type document struct {
	ID      primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Title   string             `json:"Title,omitempty" bson:"Title,omitempty"`
	Content string             `json:"Content,omitempty" bson:"Content,omitempty"`
}

type statusCode struct {
	Code    int
	Message string
}

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

func init() {
	prometheus.MustRegister(histogram)
}

func main() {
	p, mp := os.Getenv("PORT"), os.Getenv("METRICS_PORT")
	if p == "" {
		p = "8080"
	}
	if mp == "" {
		mp = "9090"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mh, _ := newMetricsHandler()

	mh.Mux = mux.NewRouter()
	mh.Mux.Handle("/metrics", mh.metrics()).Methods("GET")

	log.Printf("Serving metrics on port %s...\n", mp)
	go func() { log.Fatal(http.ListenAndServe(":"+mp, mh.Mux)) }()

	h, _ := newHandler(ctx)

	h.Mux = mux.NewRouter()
	h.Mux.HandleFunc("/", h.index).Methods("GET")
	h.Mux.HandleFunc("/health", h.health).Methods("GET")
	h.Mux.HandleFunc("/document", h.createDocument).Methods("POST")
	h.Mux.HandleFunc("/document/{id}", h.getDocument).Methods("GET")
	h.Mux.HandleFunc("/documents", h.getDocuments).Methods("GET")

	log.Printf("Listening on port %s...\n", p)
	log.Fatal(http.ListenAndServe(":"+p, h.Mux))
}

func newHandler(ctx context.Context) (*handler, error) {
	host := os.Getenv("DB_HOSTNAME")
	if host == "" {
		host = "localhost"
	}

	var uri string
	user, pwd := os.Getenv("DB_USERNAME"), os.Getenv("DB_PASSWORD")
	if user != "" && pwd != "" {
		uri = "mongodb://" + user + ":" + pwd + "@" + host + ":27017"
	} else {
		uri = "mongodb://" + host + ":27017"
	}

	h := &handler{}

	o := options.Client().ApplyURI(uri)
	h.Client, _ = mongo.Connect(ctx, o)

	log.Printf("Connecting to the database on %s:27017...\n", host)

	err := h.Client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatalf("error: %s", err)
	}

	log.Printf("Successfully connected to the database")
	return h, nil
}

func newMetricsHandler() (*handler, error) {
	mh := &handler{}
	return mh, nil
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
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
		var m string
		c, m = httpError(e)
		if c != 200 {
			log.Printf("error: " + m)
			http.Error(w, m, c)
			return
		}
	}

	resp := "Hello, world!\n"
	_, _ = io.WriteString(w, resp)
}

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, "200 OK\n")
}

func (h *handler) createDocument(w http.ResponseWriter, r *http.Request) {
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
		var m string
		c, m = httpError(e)
		if c != 200 {
			log.Printf("error: " + m)
			http.Error(w, m, c)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	var d document
	_ = json.NewDecoder(r.Body).Decode(&d)

	coll := h.Client.Database("observer").Collection("documents")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, _ := coll.InsertOne(ctx, d)
	_ = json.NewEncoder(w).Encode(res)
}

func (h *handler) getDocument(w http.ResponseWriter, r *http.Request) {
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
		var m string
		c, m = httpError(e)
		if c != 200 {
			log.Printf("error: " + m)
			http.Error(w, m, c)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	params := mux.Vars(r)
	id, _ := primitive.ObjectIDFromHex(params["id"])

	var d document
	coll := h.Client.Database("observer").Collection("documents")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := coll.FindOne(ctx, document{ID: id}).Decode(&d)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}
	_ = json.NewEncoder(w).Encode(d)
}

func (h *handler) getDocuments(w http.ResponseWriter, r *http.Request) {
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
		var m string
		c, m = httpError(e)
		if c != 200 {
			log.Printf("error: " + m)
			http.Error(w, m, c)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	var xd []document
	coll := h.Client.Database("observer").Collection("documents")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := coll.Find(ctx, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}
	defer func() { _ = cursor.Close(ctx) }()

	for cursor.Next(ctx) {
		var d document
		_ = cursor.Decode(&d)
		xd = append(xd, d)
	}
	if err := cursor.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}
	_ = json.NewEncoder(w).Encode(xd)
}

func (h *handler) metrics() http.Handler {
	return promhttp.Handler()
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
	xsc := []statusCode{
		{Code: 400, Message: "400 Bad Request"},
		{Code: 401, Message: "401 Unauthorized"},
		{Code: 403, Message: "403 Forbidden"},
		{Code: 404, Message: "404 Not Found"},
		{Code: 500, Message: "500 Internal Server Error"},
		{Code: 501, Message: "501 Not Implemented"},
		{Code: 502, Message: "502 Bad Gateway"},
		{Code: 503, Message: "503 Service Unavailable"},
		{Code: 504, Message: "504 Gateway Timeout"},
		{Code: 505, Message: "505 HTTP Version Not Supported"},
		{Code: 506, Message: "506 Variant Also Negotiates"},
		{Code: 507, Message: "507 Insufficient Storage"},
		{Code: 510, Message: "510 Not Extended"},
	}
	if e == "random" {
		// Generate random HTTP error.
		rand.Seed(time.Now().UnixNano())
		n := rand.Intn(10)
		if n == 0 {
			sc := xsc[rand.Intn(len(xsc))]
			c, m = sc.Code, sc.Message
		} else {
			c = 200
		}
	} else {
		for i, v := range xsc {
			str := strconv.Itoa(xsc[i].Code)
			if e == str {
				c, m = v.Code, v.Message
				break
			} else {
				c = 200
			}
		}
	}
	return c, m
}

func recordMetrics(start time.Time, req *http.Request, code int) {
	duration := time.Since(start)
	histogram.With(
		prometheus.Labels{
			"service": "observer",
			"code":    fmt.Sprintf("%d", code),
			"method":  req.Method,
			"path":    req.URL.Path,
		},
	).Observe(duration.Seconds())
}
