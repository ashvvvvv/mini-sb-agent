package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"mini-sb-agent/panelserver"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	adminToken := flag.String("admin-token", os.Getenv("MINI_SB_PANEL_ADMIN_TOKEN"), "admin API bearer token (or MINI_SB_PANEL_ADMIN_TOKEN)")
	flag.Parse()
	if *adminToken == "" {
		log.Fatal("missing -admin-token or MINI_SB_PANEL_ADMIN_TOKEN")
	}
	server := panelserver.New(*adminToken, panelserver.NewStore(""))
	log.Printf("mini-sb panel listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, server))
}
