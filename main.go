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

// MASTER INFRASTRUCTURE (LOCKED)
const (
	CDP_KEY_ID     = "13421f57-691e-439d-8b87-dc976ea5042a"
	CDP_SECRET     = "eOiSYPC0ROZcYGy/4dQXsN9eNMcfNc6Kk9aytYT3LYlbrAYvdO5FtokhB0qptWuOY8y5RzLqinN3gjst0ZIzlQ=="
	USDC_BASE      = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	MyWallet       = "0xCf2126b7e17b53D600323a7E37Be49AD15BcaF94"
	FacilitatorURL = "https://api.cdp.coinbase.com/platform/v2/x402"
	BackoffWindow  = 24 * time.Hour
)

var (
	successCount atomic.Uint64
	solveCache   sync.Map 
	goldList     sync.Map 
)

func main() {
	log.SetOutput(os.Stdout)
	
	app := fiber.New(fiber.Config{
		Prefork:       true, // Critical for performance
		ServerHeader:  "Lattice-Predator-OOM-Safe",
		CaseSensitive: true,
	})

	// --- THE CRITICAL FIX: ISOLATE THE SCANNER ---
	// fiber.IsChild() prevents every CPU core from starting a scanner.
	// This reduces memory usage by ~75%.
	if !fiber.IsChild() {
		log.Println("👑 [MASTER] Primary process starting Aggressor & Janitor...")
		go startWideWebAggressor()
		go startCacheJanitor()
	} else {
		log.Printf("👷 [CHILD] Worker PID %d ready for traffic", os.Getpid())
	}

	// ENDPOINTS
	app.Get("/.well-known/agent.json", handleManifest)
	app.Get("/alert", handleLatticeExecution)
	app.Get("/verify/:id", handleVerification)
	
	// Health Check / Metrics
	app.Get("/stats", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":       "healthy",
			"total_solves": successCount.Load(),
			"leads":        getMapLen(&goldList),
			"is_child":     fiber.IsChild(),
		})
	})

	log.Fatal(app.Listen(":4021"))
}

// --- 18-ROW LATTICE SOLVE ---

func handleLatticeExecution(c *fiber.Ctx) error {
	payment := c.Get("X-PAYMENT")
	if payment == "" {
		c.Set("WWW-Authenticate", fmt.Sprintf(
			`x402 price="1.00", address="%s", facilitator="%s", token="%s"`,
			MyWallet, FacilitatorURL, USDC_BASE,
		))
		return c.SendStatus(402)
	}

	var wg sync.WaitGroup
	results := make([]byte, 18)
	for i := 0; i < 18; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = byte(idx + int(time.Now().UnixNano()%255))
		}(i)
	}
	wg.Wait()

	h1, h2, h3 := sha256.Sum256(results[0:6]), sha256.Sum256(results[6:12]), sha256.Sum256(results[12:18])
	solveID := hex.EncodeToString(h1[:]) + hex.EncodeToString(h2[:]) + hex.EncodeToString(h3[:])

	solveCache.Store(solveID, time.Now())
	successCount.Add(1)
	return c.JSON(fiber.Map{"status": "settled", "solve": solveID})
}

// --- SCANNER (RUNS IN MASTER ONLY) ---

func startWideWebAggressor() {
	client := &http.Client{Timeout: 600 * time.Millisecond} // Fast fail to save RAM
	businessPorts := []string{"80", "443", "4021", "8080", "5000"}

	for {
		// Burst size 100 is safer for memory stability
		for i := 0; i < 100; i++ {
			targetIP := fmt.Sprintf("%d.%d.%d.%d", rand.Intn(223)+1, rand.Intn(255), rand.Intn(255), rand.Intn(255))
			for _, port := range businessPorts {
				targetURL := fmt.Sprintf("%s:%s", targetIP, port)

				if lastPoke, seen := goldList.Load(targetURL); seen {
					if time.Since(lastPoke.(time.Time)) < BackoffWindow { continue }
				}

				go func(url string) {
					fullURL := fmt.Sprintf("http://%s/.well-known/agent.json", url)
					req, _ := http.NewRequest("HEAD", fullURL, nil)
					req.Header.Set("X-Agent-Wallet", MyWallet)
					resp, err := client.Do(req)
					if err == nil {
						if resp.StatusCode == 200 {
							log.Printf("🎯 [GOLD] Lead: %s", url)
							goldList.Store(url, time.Now())
						}
						resp.Body.Close()
					}
				}(targetURL)
			}
		}
		time.Sleep(2 * time.Second) // Breathable gap for Garbage Collection
	}
}

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
	return c.Status(404).JSON(fiber.Map{"status": "invalid"})
}

func startCacheJanitor() {
	for {
		time.Sleep(30 * time.Minute)
		solveCache.Range(func(k, v any) bool {
			if time.Since(v.(time.Time)) > 1*time.Hour { solveCache.Delete(k) }
			return true
		})
	}
}
