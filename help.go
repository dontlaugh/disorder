package disorder

import (
	"strings"
	"sync"
)

// JoinIf combines the string pointers if they're not nil.
func JoinIf(ptrs ...*string) string {
	var once sync.Once
	var result = "ALL_NIL"
	for _, ptr := range ptrs {
		if ptr != nil {
			once.Do(func() { result = "" })
			result += *ptr + " "
		}

	}
	return strings.TrimSpace(result)
}
