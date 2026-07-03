package ecm

// getAllDefinitions returns all available ECM definitions
//
// To add a new ECM definition:
// 1. Create a new file in this package (e.g., gm_XXXXX.go)
// 2. Define a function that returns Definition (e.g., gmXXXXX())
// 3. Add it to the slice returned by this function
func getAllDefinitions() []Definition {
	return []Definition{
		gm1227747(),
		// Add more ECM definitions here as they are implemented
		// Example: gm1234567(),
	}
}
