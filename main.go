package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
)

// MASTER INFRASTRUCTURE
const (
	CDP_KEY_ID     = "13421f57-691e-439d-8b87-dc976ea5042a"
	CDP_SECRET     = "eOiSYPC0ROZcYGy/4dQXsN9eNMcfNc6Kk9aytYT3LYlbrAYvdO5FtokhB0qptWuOY8y5RzLqinN3gjst0ZIzlQ=="
	USDC_BASE      = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	MyWallet       = "0xCf2126b7e17b53D600323a7E37Be49AD15BcaF94"
	FacilitatorURL = "https://api.cdp.coinbase.com/platform/v2/x402"
	
	// REDUCED CONCURRENCY FOR STABILITY
	MaxScannerWorkers = 50 
	BackoffWindow     = 24 * time.Hour
)

var (
	successCount atomic.Uint64
	solveCache   sync.Map 
	goldList     sync.Map 
)

func main() {
	app := fiber.New(fiber.Config{
		Prefork:       true, 
		ServerHeader:  "Lattice-Predator-V5",
	})

	// ONLY MASTER RUNS THE BACKGROUND ENGINE
	if !fiber.IsChild() {
		log.Println("👑 [MASTER] Starting Worker-Limited Aggressor...")
		go startWideWebAggressor()
		go startCacheJanitor()
	}

	// ENDPOINTS
	app.Get("/.well-known/agent.json", handleManifest)
	app.Get("/alert", handleLatticeExecution)
	app.Get("/verify/:id", handleVerification)
	app.Get("/stats", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"total_solves": successCount.Load(),
			"leads":        getMapLen(&goldList),
		})
	})

	log.Fatal(app.Listen(":4021"))
}

// --- NEW STABILIZED SCANNER (Worker Pool Pattern) ---

func startWideWebAggressor() {
	jobs := make(chan string, 100)
	client := &http.Client{Timeout: 500 * time.Millisecond} // Faster timeout = lower RAM

	// Start a fixed number of workers - No more infinite goroutines
	for w := 1; w <= MaxScannerWorkers; w++ {
		go func() {
			for target := range jobs {
				fullURL := fmt.Sprintf("http://%s/.well-known/agent.json", target)
				req, _ := http.NewRequest("HEAD", fullURL, nil)
				req.Header.Set("X-Agent-Wallet", MyWallet)
				resp, err := client.Do(req)
				if err == nil {
					if resp.StatusCode == 200 {
						log.Printf("🎯 [GOLD] %s", target)
						goldList.Store(target, time.Now())
					}
					resp.Body.Close()
				}
			}
		}()
	}

	// Feed the workers at a controlled rate
	ports := []string{"80", "443", "4021", "8080", "5000"}
	for {
		ip := fmt.Sprintf("%d.%d.%d.%d", rand.Intn(223)+1, rand.Intn(255), rand.Intn(255), rand.Intn(255))
		for _, port := range ports {
			targetURL := fmt.Sprintf("%s:%s", ip, port)
			
			// Skip if already in Gold List (24hr backoff)
			if last, seen := goldList.Load(targetURL); seen {
				if time.Since(last.(time.Time)) < BackoffWindow { continue }
			}
			
			jobs <- targetURL
		}
		// Throttle the IP generator so it doesn't overwhelm the workers
		time.Sleep(10 * time.Millisecond) 
	}
}

// --- PRODUCT LOGIC (18-ROW) ---

func handleLatticeExecution(c *fiber.Ctx) error {
	if c.Get("X-PAYMENT") == "" {
		c.Set("WWW-Authenticate", fmt.Sprintf(`x402 price="1.00", address="%s", facilitator="%s", token="%s"`, MyWallet, FacilitatorURL, USDC_BASE))
		return c.SendStatus(402)
	}
	
	// Synchronous solve to prevent CPU spikes from goroutine spawning
	results := make([]byte, 18)
	for i := 0; i < 18; i++ {
		results[i] = byte(i + int(time.Now().UnixNano()%255))
	}
	h1, h2, h3 := sha256.Sum256(results[0:6]), sha256.Sum256(results[6:12]), sha256.Sum256(results[12:18])
	solveID := hex.EncodeToString(h1[:]) + hex.EncodeToString(h2[:]) + hex.EncodeToString(h3[:])

	solveCache.Store(solveID, time.Now())
	successCount.Add(1)
	return c.JSON(fiber.Map{"status": "settled", "solve": solveID})
}

// --- HELPERS ---

func getMapLen(m *sync.Map) int {
	len := 0
	m.Range(func(k, v any) bool { len++; return true })
	return len
}

func handleManifest(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"name": "Lattice-Fast-Solve", "cost": "1.00 USDC"})
}

func handleVerification(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, valid := solveCache.Load(id); valid { return c.JSON(fiber.Map{"status": "verified"}) }
	return c.Status(404).
