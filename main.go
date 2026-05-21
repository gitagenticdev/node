package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	nodeName   = os.Getenv("NODE_NAME")
	nodeRegion = os.Getenv("NODE_REGION")
	nodeFlag   = os.Getenv("NODE_FLAG")
	peers      = strings.Split(os.Getenv("PEERS"), ",")
	nodeDID    string
	nodeKey    ed25519.PrivateKey
	version    = "0.3.8"
)

// In-memory state
var state = &nodeState{
	Repos:  make(map[string]*Repo),
	Events: make([]Event, 0),
}

type nodeState struct {
	mu     sync.RWMutex
	Repos  map[string]*Repo
	Events []Event
	Writes int
	Gossip int
}

type Repo struct {
	Name    string `json:"name"`
	DID     string `json:"did"`
	CID     string `json:"cid"`
	Objects int    `json:"objects"`
	Updated string `json:"updated"`
}

type Event struct {
	Type string `json:"type"`
	Repo string `json:"repo"`
	DID  string `json:"did"`
	CID  string `json:"cid"`
	Ts   string `json:"ts"`
	From string `json:"from"`
}

func main() {
	if nodeName == "" {
		nodeName = "node.gitagentic.dev"
	}
	if nodeRegion == "" {
		nodeRegion = "US-EAST"
	}
	if nodeFlag == "" {
		nodeFlag = "🇺🇸"
	}

	// Generate node DID
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	nodeKey = priv
	mc := append([]byte{0xed, 0x01}, pub...)
	nodeDID = "did:key:z" + base58Encode(mc)

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	mux := http.NewServeMux()

	// Node info (root)
	mux.HandleFunc("/", handleInfo)
	mux.HandleFunc("/health", handleHealth)

	// Git operations
	mux.HandleFunc("/api/v1/repos", handleRepos)
	mux.HandleFunc("/api/v1/stats", handleStats)
	mux.HandleFunc("/api/v1/peers", handlePeers)
	mux.HandleFunc("/api/v1/events", handleEvents)
	mux.HandleFunc("/api/v1/agents", handleAgents)

	// Gossip (HTTP-based peer sync)
	mux.HandleFunc("/gossip/push", handleGossipReceive)
	mux.HandleFunc("/gossip/sync", handleGossipSync)

	// Start gossip background
	go gossipLoop()

	log.Printf("gitagentic-node %s starting", version)
	log.Printf("  name:   %s", nodeName)
	log.Printf("  region: %s %s", nodeFlag, nodeRegion)
	log.Printf("  DID:    %s", nodeDID[:40]+"...")
	log.Printf("  port:   %s", port)
	log.Printf("  peers:  %v", peers)

	log.Fatal(http.ListenAndServe(":"+port, corsMiddleware(mux)))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"name":       nodeName,
		"did":        nodeDID,
		"version":    version,
		"region":     nodeRegion,
		"flag":       nodeFlag,
		"network":    "alpha",
		"protocols":  []string{"git-smart-http", "gossip-http", "mcp"},
		"p2p_peer_id": nodeDID[8:20],
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	repos := len(state.Repos)
	writes := state.Writes
	state.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "repos": repos, "writes": writes, "version": version})
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]any{
		"repos":   len(state.Repos),
		"agents":  0,
		"pushes":  state.Writes,
		"version": version,
	})
}

func handleRepos(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var req struct {
			Name string `json:"name"`
			DID  string `json:"did"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name == "" {
			http.Error(w, "name required", 400)
			return
		}
		if req.DID == "" {
			req.DID = nodeDID
		}
		state.mu.Lock()
		state.Repos[req.Name] = &Repo{Name: req.Name, DID: req.DID, Updated: time.Now().UTC().Format(time.RFC3339)}
		state.Writes++
		state.mu.Unlock()
		gossipBroadcast(Event{Type: "repo_create", Repo: req.Name, DID: req.DID, Ts: time.Now().UTC().Format(time.RFC3339), From: nodeName})
		json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": req.Name})
		return
	}
	state.mu.RLock()
	repos := make([]*Repo, 0, len(state.Repos))
	for _, r := range state.Repos {
		repos = append(repos, r)
	}
	state.mu.RUnlock()
	json.NewEncoder(w).Encode(repos)
}

func handlePeers(w http.ResponseWriter, r *http.Request) {
	out := []map[string]any{}
	for _, p := range peers {
		if p == "" {
			continue
		}
		out = append(out, map[string]any{"url": p, "status": "connected"})
	}
	json.NewEncoder(w).Encode(out)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	n := len(state.Events)
	start := 0
	if n > 50 {
		start = n - 50
	}
	json.NewEncoder(w).Encode(state.Events[start:])
}

func handleAgents(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode([]map[string]any{
		{"did": nodeDID, "capabilities": []string{"git:push", "git:fetch", "gossip:relay"}, "trust_score": 1.0},
	})
}

// ─── Gossip ──────────────────────────────────────────────────────────────────

func handleGossipReceive(w http.ResponseWriter, r *http.Request) {
	var event Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad event", 400)
		return
	}
	state.mu.Lock()
	state.Events = append(state.Events, event)
	state.Gossip++
	if event.Type == "push" && event.Repo != "" {
		if repo, ok := state.Repos[event.Repo]; ok {
			repo.CID = event.CID
			repo.Objects++
			repo.Updated = event.Ts
		} else {
			state.Repos[event.Repo] = &Repo{Name: event.Repo, DID: event.DID, CID: event.CID, Objects: 1, Updated: event.Ts}
		}
	}
	state.mu.Unlock()
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func handleGossipSync(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]any{
		"node":   nodeName,
		"did":    nodeDID,
		"repos":  len(state.Repos),
		"writes": state.Writes,
		"gossip": state.Gossip,
	})
}

func gossipBroadcast(event Event) {
	data, _ := json.Marshal(event)
	for _, peer := range peers {
		if peer == "" {
			continue
		}
		go func(url string) {
			http.Post(url+"/gossip/push", "application/json", bytes.NewReader(data))
		}(peer)
	}
}

func gossipLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		for _, peer := range peers {
			if peer == "" {
				continue
			}
			go func(url string) {
				resp, err := http.Get(url + "/gossip/sync")
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var sync map[string]any
				json.NewDecoder(resp.Body).Decode(&sync)
				state.mu.Lock()
				state.Gossip++
				state.mu.Unlock()
			}(peer)
		}
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

const b58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(input []byte) string {
	var result []byte
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)
	for x.Cmp(zero) != 0 {
		x.DivMod(x, base, mod)
		result = append(result, b58Alphabet[mod.Int64()])
	}
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, b58Alphabet[0])
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// keep import alive
var _ = hex.EncodeToString
var _ = fmt.Sprintf
