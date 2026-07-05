// Command vapid-keys prints a fresh VAPID keypair for Web Push, to paste into
// VAPID_PUBLIC_KEY / VAPID_PRIVATE_KEY. See `make vapid-keys`.
package main

import (
	"fmt"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		log.Fatalf("generate VAPID keys: %v", err)
	}
	fmt.Printf("VAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\n", pub, priv)
}
