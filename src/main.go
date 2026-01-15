package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrs := []string{
		"redis-01.jg88.sat:7000",
		"redis-01.jg88.sat:7001",
		"redis-01.jg88.sat:7002",
		"redis-01.jg88.sat:7003",
	}
	printResolved(addrs)

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: addrs,
		// Username: "default", // if ACL enabled (Redis 6+)
		// Password: "secret",  // if required
		// TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, // if using TLS
	})
	defer func() { _ = rdb.Close() }()

	if err := rdb.ForEachShard(ctx, func(ctx context.Context, shard *redis.Client) error {
		return shard.Ping(ctx).Err()
	}); err != nil {
		log.Fatalf("failed to connect to cluster: %v", err)
	}
	fmt.Println("✅ connected to redis cluster")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello! Updated 2025-09-03 11:09:00")
		printResolved(addrs)
	})

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func printResolved(addrs []string) {
	for _, a := range addrs {
		host, port, err := net.SplitHostPort(a)
		if err != nil {
			fmt.Printf("addr=%s (parse error: %v)\n", a, err)
			continue
		}

		ips, err := net.LookupIP(host)
		if err != nil {
			fmt.Printf("addr=%s -> DNS lookup failed: %v\n", a, err)
			continue
		}

		fmt.Printf("addr=%s resolves to:\n", a)
		for _, ip := range ips {
			fmt.Printf("  - %s:%s\n", ip.String(), port)
		}
	}
}
