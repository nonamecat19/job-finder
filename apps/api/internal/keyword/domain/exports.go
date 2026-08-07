package domain

var TokenRe = tokenRe

func LowerASCII(s string) string { return lowerASCII(s) }

func FilterStopwords(tokens []string) []string { return filterStopwords(tokens) }

func Stem(s string) string { return stem(s) }
