package main

import (
	"fmt"

	"radius-parser/internal/radius"
)

func main() {
	fmt.Println(radius.GetRadiusAttributeName(1))
	fmt.Println(radius.GetRadiusAttributeName(40))
	fmt.Println(radius.GetRadiusAttributeName(999))
}