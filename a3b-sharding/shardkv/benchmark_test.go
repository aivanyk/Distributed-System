package shardkv

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"
)

const NumRequests = 1000

func TestBenchmaking3C_A(t *testing.T) {
	fmt.Printf("Test: Benchmark A ...\n")
	cfg := make_config(t, 3, false, -1)
	runBenchmark(cfg, "./data/A.txt", 1, t)

}

func TestBenchmaking3C_B(t *testing.T) {
	fmt.Printf("Test: Benchmark B ...\n")
	cfg := make_config(t, 3, false, -1)
	runBenchmark(cfg, "./data/B.txt", 2, t)

}

func TestBenchmaking3C_C(t *testing.T) {
	fmt.Printf("Test: Benchmark C ...\n")
	cfg := make_config(t, 3, false, -1)
	runBenchmark(cfg, "./data/C.txt", 2, t)
}

func runBenchmark(cfg *config, dataPath string, nShards int, t *testing.T) {

	defer cfg.cleanup()

	nKeys := 6
	keys, err := ReadFileToSlice(dataPath)

	if err != nil {
		t.Fatalf("Error Reading data %v\n", err)
	}

	time.Sleep(time.Second)

	for i := 0; i < nShards; i++ {
		cfg.join(i)
	}

	time.Sleep(1 * time.Second)

	var clients []*Clerk
	fmt.Printf("Creating clients...\n")
	for i := 0; i < nKeys; i++ {
		ck := cfg.makeClientBenchmark()
		clients = append(clients, ck)
	}

	var wg sync.WaitGroup
	wg.Add(nKeys)
	fmt.Printf("Starting Benchmark...\n")
	for i := 0; i < nKeys; i++ {
		go func(key string, idx int) {
			ck := clients[idx]
			va := randstring(5)
			ck.Put(key, va)

			for j := 0; j < NumRequests; j++ {
				if rand.Float64() < 0.75 {
					ck.Put(key, randstring(5))
				} else {
					ck.Get(key)
				}
			}

			wg.Done()
		}(keys[i], i)
	}
	wg.Wait()
	fmt.Printf("  ... Done\n")
}

func ReadFileToSlice(filePath string) ([]string, error) {
	var result []string

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Use a scanner to read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		result = append(result, scanner.Text())
	}

	// Check for errors during scanning
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
