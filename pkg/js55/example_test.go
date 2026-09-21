// SPDX-License-Identifier: BUSL-1.1
package js55_test

import (
	"fmt"

	"github.com/hazyhaar/js55/pkg/js55"
)

func Example_basic() {
	iso, err := js55.NewIsolate(js55.Config{
		MaxMemoryBytes: 1024 * 1024, // 1 MB quota
		StepLimit:      100_000,     // 100k instructions quota
	})
	if err != nil {
		panic(err)
	}
	defer iso.Close()

	val, err := iso.Eval(`
		const numbers = [1, 2, 3, 4, 5];
		numbers.map(x => x * 2).reduce((a, b) => a + b, 0);
	`)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Result: %v\n", val)
	// Output: Result: 30
}
