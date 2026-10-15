package main

import "os"

func main() {
	_ = os.Getenv("TOKEN")
	_, _ = os.LookupEnv("REGION")
}
