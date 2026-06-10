package cases

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Case string

const (
	Camel    Case = "camel"
	Kebab    Case = "kebab"
	Snake    Case = "snake"
	Pascal   Case = "pascal"
	Upper    Case = "upper"
	Lower    Case = "lower"
	Constant Case = "constant"
)

func Convert(m map[string]interface{}, target Case) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for key, value := range m {
		convertedKey := ConvertKey(key, target)
		result[convertedKey] = convertValue(value, target)
	}
	return result
}

func convertValue(value interface{}, target Case) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return Convert(v, target)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = convertValue(item, target)
		}
		return result
	default:
		return value
	}
}

func ConvertKey(key string, target Case) string {
	words := splitWords(key)
	if len(words) == 0 {
		return key
	}
	switch target {
	case Camel:
		return toCamel(words)
	case Kebab:
		return toKebab(words)
	case Snake:
		return toSnake(words)
	case Pascal:
		return toPascal(words)
	case Upper:
		return strings.ToUpper(strings.Join(words, ""))
	case Lower:
		return strings.ToLower(strings.Join(words, ""))
	case Constant:
		return strings.ToUpper(toSnake(words))
	default:
		return key
	}
}

func splitWords(s string) []string {
	var words []string
	var current strings.Builder

	for i, r := range s {
		if r == '-' || r == '_' || r == ' ' || r == '.' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		if unicode.IsUpper(r) && current.Len() > 0 {
			prev, _ := utf8.DecodeLastRuneInString(current.String())
			if !unicode.IsUpper(prev) || (i+1 < len(s) && unicode.IsLower(rune(s[i+1]))) {
				if current.Len() > 0 {
					words = append(words, current.String())
					current.Reset()
				}
			}
		}
		current.WriteRune(unicode.ToLower(r))
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func capitalize(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}

func toCamel(words []string) string {
	result := strings.Builder{}
	for i, word := range words {
		if i == 0 {
			result.WriteString(word)
		} else {
			result.WriteString(capitalize(word))
		}
	}
	return result.String()
}

func toKebab(words []string) string {
	return strings.Join(words, "-")
}

func toSnake(words []string) string {
	return strings.Join(words, "_")
}

func toPascal(words []string) string {
	result := strings.Builder{}
	for _, word := range words {
		result.WriteString(capitalize(word))
	}
	return result.String()
}