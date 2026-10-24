package gallery

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os/exec"
	"time"
)

// KOROMAN 1.0.16, vendored unchanged from otakuict/koroman. MIT license beside asset.
//
//go:embed assets/koroman.mjs
var koromanSource string

func romanizeKorean(ctx context.Context, names []string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	input, err := json.Marshal(names)
	if err != nil {
		return nil, err
	}
	// Source is trusted embedded code; user input travels only over stdin.
	script := koromanSource + "\nimport fs from 'node:fs'; const names=JSON.parse(fs.readFileSync(0,'utf8')); console.log(JSON.stringify(names.map(name=>romanize(name,{casingOption:'capitalize-word'}))));"
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", script)
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if err != nil {
		return nil, errors.New("KOROMAN is unavailable; ensure Node.js is installed on the backend")
	}
	var result []string
	if json.Unmarshal(output, &result) != nil || len(result) != len(names) {
		return nil, errors.New("Invalid KOROMAN result")
	}
	return result, nil
}
