// Test-only transport for original migration oracle comparisons.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"rumsdlc/internal/migration"
)

func main() {
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 65536), 16*1024*1024)
	encoder := json.NewEncoder(os.Stdout)
	for reader.Scan() {
		var options migration.Options
		if e := json.Unmarshal(reader.Bytes(), &options); e != nil {
			encoder.Encode(map[string]any{"error": e.Error()})
			continue
		}
		result, e := migration.Prepare(options)
		if e != nil {
			encoder.Encode(map[string]any{"error": e.Error()})
			continue
		}
		files := map[string]string{}
		for name, data := range result.Files {
			files[name] = hex.EncodeToString(data)
		}
		encoder.Encode(map[string]any{"state": result.State, "files": files, "report": result.Report})
	}
}
