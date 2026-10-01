// Baseline: one streaming POST with net/http and encoding/json only.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func main() {
	body, _ := json.Marshal(map[string]any{"model": "m", "stream": true, "messages": []any{map[string]any{"role": "user", "content": "hi"}}})
	resp, err := http.Post(os.Args[1]+"/v1/chat/completions", "application/json", strings.NewReader(string(body)))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		fmt.Println(sc.Text())
	}
}
