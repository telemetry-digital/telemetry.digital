// totpcode prints the current TOTP code for a base32 secret (test helper for scripted logins).
//
//	go run ./tools/totpcode -secret JBSWY3DPEHPK3PXP
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/pquerna/otp/totp"
)

func main() {
	secret := flag.String("secret", "", "base32 TOTP secret")
	flag.Parse()
	if *secret == "" {
		log.Fatal("-secret required")
	}
	code, err := totp.GenerateCode(*secret, time.Now())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(code)
}
