package main

import (
	"fmt"

	"github.com/huncoding/payment-processor-course/pkg/fsm"
)

func main() {
	fmt.Println("Fraud service starting...")
	fmt.Print(fsm.ErrNoTransition)
}
