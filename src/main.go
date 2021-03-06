package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/mailgun/groupcache"
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

type server struct {
	router *mux.Router
	client *mongo.Client
	pool   *groupcache.HTTPPool
	group  *groupcache.Group
}

type Document struct {
	ID      primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Title   string             `json:"Title,omitempty" bson:"Title,omitempty"`
	Content string             `json:"Content,omitempty" bson:"Content,omitempty"`
}

type StatusCode struct {
	Code    int
	Message string
}

type CacheStats struct {
	Group     groupcache.Stats      `json:"Group,omitempty" bson:"Group,omitempty"`
	MainCache groupcache.CacheStats `json:"MainCache,omitempty" bson:"MainCache,omitempty"`
	HotCache  groupcache.CacheStats `json:"HotCache,omitempty" bson:"HotCache,omitempty"`
}

func main() {
	// Create a new server for metrics, add the router, and serve.
	sm, _ := newServerMetrics()
	sm.newServerMetricsRouter()
	go func() { log.Fatal(http.ListenAndServe(":9090", sm.router)) }()
	// Create a new server for metrics, add the router, and serve.
	s, _ := newServer()
	s.newServerRouter()
	// Create a new cache pool and group.
	s.newGroupCache()
	log.Printf("Serving...\n")
	log.Fatal(http.ListenAndServe(":8080", s.router))
}

func newServer() (*server, error) {
	s := &server{}
	s.connectClient()
	return s, nil
}

func newServerMetrics() (*server, error) {
	s := &server{}
	return s, nil
}

func (s *server) newServerRouter() {
	s.router = mux.NewRouter()
	s.router.HandleFunc("/health", s.health).Methods("GET")
	s.router.HandleFunc("/stats", s.stats).Methods("GET")
	s.router.HandleFunc("/document", s.insertOne).Methods("POST")
	s.router.HandleFunc("/document/{id}", s.findOne).Methods("GET")
	s.router.HandleFunc("/documents", s.find).Methods("GET")
}

func (s *server) newServerMetricsRouter() {
	s.router = mux.NewRouter()
	s.router.Handle("/metrics", s.metrics()).Methods("GET")
}

func (s *server) connectClient() {
	log.Printf("Connecting to the database...\n")
	host := os.Getenv("DB_HOSTNAME")
	if host == "" {
		host = "localhost"
	}
	user, pwd := os.Getenv("DB_USERNAME"), os.Getenv("DB_PASSWORD")
	uri := "mongodb://" + user + ":" + pwd + "@" + host + ":27017"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opt := options.Client().ApplyURI(uri)
	s.client, _ = mongo.Connect(ctx, opt)
	err := s.client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatalf("error: %s", err)
	}
}

func (s *server) newGroupCache() {
	peers := os.Getenv("CACHE_PEERS")
	if peers == "" {
		peers = "http://localhost:8080"
	}
	s.pool = groupcache.NewHTTPPoolOpts(peers, &groupcache.HTTPPoolOptions{})
	s.group = groupcache.NewGroup("observer", 10485760, groupcache.GetterFunc(
		func(ctx groupcache.Context, id string, dest groupcache.Sink) error {
			log.Printf("Caching %s... \n", id)
			pid, _ := primitive.ObjectIDFromHex(id)
			var d Document
			coll := s.client.Database("observer").Collection("documents")
			ctxClient, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := coll.FindOne(ctxClient, Document{ID: pid}).Decode(&d)
			if err != nil {
				return err
			}
			buff := new(bytes.Buffer)
			_ = json.NewEncoder(buff).Encode(d)
			if err = dest.SetBytes(buff.Bytes(), time.Now().Add(time.Minute*5)); err != nil {
				return err
			}
			return nil
		},
	))
}

func (s *server) latency(l string) {
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
}

func (s *server) httpError(e string) (int, string) {
	xsc := []StatusCode{
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
	var c = 200
	var m string
	if e == "random" {
		rand.Seed(time.Now().UnixNano())
		n := rand.Intn(10)
		if n == 0 {
			sc := xsc[rand.Intn(len(xsc))]
			c, m = sc.Code, sc.Message
		}
	} else {
		ec, _ := strconv.Atoi(e)
		for _, v := range xsc {
			if ec == v.Code {
				c, m = v.Code, v.Message
			}
		}
	}
	return c, m
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	_, _ = io.WriteString(w, "200 OK\n")
}

func (s *server) metrics() http.Handler {
	return promhttp.Handler()
}

func (s *server) stats(w http.ResponseWriter, _ *http.Request) {
	var xcs CacheStats
	xcs.Group = s.group.Stats
	xcs.MainCache = s.group.CacheStats(1)
	xcs.HotCache = s.group.CacheStats(2)
	_ = json.NewEncoder(w).Encode(xcs)
	// TODO: JSON formatting or the output.
}

func (s *server) insertOne(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)
	// Insert optional latency and/or HTTP error.
	l := r.URL.Query().Get("latency")
	if len(l) > 0 {
		s.latency(l)
	}
	e := r.URL.Query().Get("error")
	if len(e) > 0 {
		c, m := s.httpError(e)
		if c != 200 {
			http.Error(w, m, c)
			log.Printf("error: " + m)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	var d Document
	_ = json.NewDecoder(r.Body).Decode(&d)
	coll := s.client.Database("observer").Collection("documents")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, _ := coll.InsertOne(ctx, d)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *server) findOne(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)
	// Insert optional latency and/or HTTP error.
	l := r.URL.Query().Get("latency")
	if len(l) > 0 {
		s.latency(l)
	}
	e := r.URL.Query().Get("error")
	if len(e) > 0 {
		c, m := s.httpError(e)
		if c != 200 {
			http.Error(w, m, c)
			log.Printf("error: " + m)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id := vars["id"]
	var b []byte
	err := s.group.Get(nil, id, groupcache.AllocatingByteSliceSink(&b))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_, _ = w.Write(b)
}

func (s *server) find(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s request to %s\n", r.Method, r.RequestURI)
	// Insert optional latency and/or HTTP error.
	l := r.URL.Query().Get("latency")
	if len(l) > 0 {
		s.latency(l)
	}
	e := r.URL.Query().Get("error")
	if len(e) > 0 {
		c, m := s.httpError(e)
		if c != 200 {
			http.Error(w, m, c)
			log.Printf("error: " + m)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	var xd []Document
	coll := s.client.Database("observer").Collection("documents")
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
		var d Document
		_ = cursor.Decode(&d)
		xd = append(xd, d)
	}
	if err = cursor.Err(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{ "message": "` + err.Error() + `" }`))
		return
	}
	_ = json.NewEncoder(w).Encode(xd)
}
