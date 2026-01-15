package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const publicHost = "redis-01.jg88.sat" // or "103.148.239.13"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{
			publicHost + ":7000", // seed only is enough
		},

		// Key fix: rewrite private node IPs advertised by the cluster
		NewClient: func(opt *redis.Options) *redis.Client {
			host, port, err := net.SplitHostPort(opt.Addr)
			if err == nil {
				if strings.HasPrefix(host, "172.16.") {
					opt.Addr = net.JoinHostPort(publicHost, port)
				}
			}
			return redis.NewClient(opt)
		},

		DialTimeout:  10 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	})
	defer func() { _ = rdb.Close() }()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to connect to cluster: %v", err)
	}
	fmt.Println("✅ connected to redis cluster")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
	})

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
