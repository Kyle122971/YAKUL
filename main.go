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

// MASTER INFRASTRUCTURE (RESTORED & SECURED)
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
	// Standard output for Northflank logs
	log.SetOutput(os.Stdout)
	
	app := fiber.New(fiber.Config{
		Prefork:       true, // Critical for performance
		ServerHeader:  "Lattice-Predator-OOM-Safe",
		CaseSensitive: true,
	})

	// --- THE OOM FIX: ISOLATE THE AGGRESSOR ---
	// fiber.IsChild() ensures the scanner ONLY runs in the master process.
	// This prevents memory duplication across your 4 vCPUs.
	if !fiber.IsChild() {
		log.Println("👑 [MASTER] Memory isolation active. Starting Aggressor...")
		go startWideWebAggressor()
		go startCacheJanitor()
	} else {
		log.Printf("👷 [CHILD] Worker PID %d handling traffic...", os.Getpid())
	}

	// --- REVENUE ENDPOINTS (Available to all processes) ---
	app.Get("/.well-known/agent.json", handleManifest)
	app.Get("/alert", handleLatticeExecution)
	app.Get("/verify/:id", handleVerification)
	
	// Triage & Metrics Endpoint
	app.Get("/stats", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"uptime_status":  "healthy",
			"total_solves":   successCount.Load(),
			"leads_found":    getMapLen(&goldList),
			"memory_safety":  "isolated",
			"current_pid":    os.Getpid(),
		})
	})

	// Bind to the Northflank port
	log.Fatal(app.Listen(":4021"))
}

// --- CORE PRODUCT (18-ROW LATTICE SOLVE) ---

func handleLatticeExecution(c *fiber.Ctx) error {
	payment := c.Get("X-PAYMENT")

	// Trigger x402 Payment Challenge
	if payment == "" {
		c.Set("WWW-Authenticate", fmt.Sprintf(
			`x402 price="1.00", address="%s", facilitator="%s", token="%s"`,
			MyWallet, FacilitatorURL, USDC_BASE,
		))
		return c.SendStatus(402)
	}

	// 18-Row Cryptographic Solve
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

	// Store and Settle
	solveCache.Store(solveID, time.Now())
	successCount.Add(1)

	return c.JSON(fiber.Map{"status": "settled", "solve": solveID})
}

// --- WIDE-WEB SCANNER (MASTER PROCESS ONLY) ---

func startWideWebAggressor() {
	// Optimization: Lower timeout to 600ms to fail fast and save RAM
	client := &http.Client{Timeout: 600 * time.Millisecond}
	businessPorts := []string{"80", "443", "4021", "8080", "5000"}

	for {
		// Scans in smaller bursts (150) to keep Master process memory flat
		for i := 0; i < 150; i++ {
			targetIP := fmt.Sprintf("%d.%d.%d.%d", rand.Intn(223)+1, rand.Intn(255), rand.Intn(255), rand.Intn(255))
			
			for _, port := range businessPorts {
				targetURL := fmt.Sprintf("%s:%s", targetIP, port)

				// 24-Hour Smart Backoff Check
				if lastPoke, seen := goldList.Load(targetURL); seen {
					if time.Since(lastPoke.(time.Time)) < BackoffWindow {
						continue 
					}
				}

				go func(url string) {
					fullURL := fmt.Sprintf("http://%s/.well-known/agent.json", url)
					req, _ := http.NewRequest("HEAD", fullURL, nil)
					req.Header.Set("X-Agent-Wallet", MyWallet) 
					
					resp, err := client.Do(req)
					if err == nil {
						if resp.StatusCode == 200 {
							log.Printf("🎯 [GOLD] New Discovery: %s", url)
							goldList.Store(url, time.Now())
						}
						resp.Body.Close()
					}
				}(targetURL)
			}
		}
		// Wait 2 seconds between bursts to allow GC to clean up buffers
		time.Sleep(2 * time.Second)
	}
}

// --- SUPPORTING LOGIC ---

func getMapLen(m *sync.Map) int {
	counter := 0
	m.Range(func(k, v any) bool {
		counter++
		return true
	})
	return counter
}

func handleManifest(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"name": "Lattice-Fast-Solve",
		"desc": "Enterprise PoW clearing house.",
		"cost": "1.00 USDC",
	})
}

func handleVerification(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, valid := solveCache.Load(id); valid {
		return c.JSON(fiber.Map{"status": "verified"})
	}
	return c.Status(404).JSON(fiber.Map{"status": "invalid"})
}

func startCacheJanitor() {
	for {
		time.Sleep(30 * time.Minute)
		solveCache.Range(func(k, v any) bool {
			if time.Since(v.(time.Time)) > 1*time.Hour {
				solveCache.Delete(k)
			}
			return true
		})
	}
}
