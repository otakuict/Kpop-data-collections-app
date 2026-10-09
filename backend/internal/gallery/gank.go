package gallery

import (
	"encoding/json"
	"regexp"
	"strings"
)

var nuxtCall = regexp.MustCompile(`(?s)window\.__NUXT__\s*=\s*\(function\(([^)]*)\)\s*\{.*\}\s*\((.*)\)\s*\);?\s*$`)
var nuxtPost = regexp.MustCompile(`(?s)\bpost\s*:\s*\{\s*id\s*:\s*([^,{}]+),(.*?)\bpostMedia\s*:\s*\[([^\]]*)\]`)
var nuxtMedia = regexp.MustCompile(`\{([^{}]*)\}`)
var nuxtURL = regexp.MustCompile(`(?:^|,)\s*url\s*:\s*("(?:\\.|[^"\\])*"|[A-Za-z_$][\w$]*)\s*(?:,|$)`)
var nuxtType = regexp.MustCompile(`(?:^|,)\s*type\s*:\s*("(?:\\.|[^"\\])*"|[A-Za-z_$][\w$]*)\s*(?:,|$)`)
var nuxtAccess = regexp.MustCompile(`(?:^|,)\s*accessType\s*:\s*("(?:\\.|[^"\\])*"|[A-Za-z_$][\w$]*)\s*(?:,|$)`)

// Read only JSON literal arguments and image references; never execute the Nuxt script.
func gankNuxtPostImages(script, postID string) []string {
	call := nuxtCall.FindStringSubmatch(script)
	if len(call) != 3 {
		return nil
	}
	params := strings.Split(call[1], ",")
	var args []json.RawMessage
	if json.Unmarshal([]byte("["+call[2]+"]"), &args) != nil || len(params) != len(args) {
		return nil
	}
	aliases := map[string]string{}
	for i, param := range params {
		var value string
		if json.Unmarshal(args[i], &value) == nil {
			aliases[strings.TrimSpace(param)] = value
		}
	}
	resolve := func(raw string) string {
		var value string
		if json.Unmarshal([]byte(strings.TrimSpace(raw)), &value) == nil {
			return value
		}
		return aliases[strings.TrimSpace(raw)]
	}
	urls := []string{}
	for _, post := range nuxtPost.FindAllStringSubmatch(script, -1) {
		access := nuxtAccess.FindStringSubmatch(post[2])
		if resolve(post[1]) != postID || len(access) != 2 || resolve(access[1]) != "public" {
			continue
		}
		for _, media := range nuxtMedia.FindAllStringSubmatch(post[3], -1) {
			url := nuxtURL.FindStringSubmatch(media[1])
			typeRef := nuxtType.FindStringSubmatch(media[1])
			if len(url) == 2 && len(typeRef) == 2 && resolve(typeRef[1]) == "image" {
				urls = append(urls, resolve(url[1]))
			}
		}
	}
	return urls
}
