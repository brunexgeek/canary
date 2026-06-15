package main

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

/*
var palette = []string{
	"#E57373", // red
	"#F06292", // pink
	"#BA68C8", // purple
	"#64B5F6", // blue
	"#4DB6AC", // teal
	"#81C784", // green
	"#FFD54F", // amber
	"#FF8A65", // orange
}*/

var palette = []string{
	"#1F6FEB",
	"#238636",
	"#8957E5",
	"#DA3633",
	"#9E6A03",
	"#0969DA",
	"#8250DF",
	"#2DA44E",
}

func GenerateIdenticon(seed string) string {
	hash := sha256.Sum256([]byte(seed))

	color := palette[int(hash[0])%len(palette)]

	const (
		gridSize = 5
		cellSize = 20
		padding  = 10
	)

	canvasSize := gridSize*cellSize + padding*2

	var svg strings.Builder

	svg.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		canvasSize,
		canvasSize,
		canvasSize,
		canvasSize,
	))

	// Background
	svg.WriteString(
		`<rect width="100%" height="100%" fill="#ffffff"/>`,
	)

	// Use remaining hash bits for the pattern.
	bitIndex := 8

	for row := 0; row < gridSize; row++ {
		for col := 0; col < 3; col++ {
			byteIndex := bitIndex / 8
			mask := byte(1 << (bitIndex % 8))

			filled := (hash[byteIndex] & mask) != 0

			if filled {
				cols := []int{col}

				// Mirror left side onto right side.
				if col < 2 {
					cols = append(cols, gridSize-1-col)
				}

				for _, actualCol := range cols {
					x := padding + actualCol*cellSize
					y := padding + row*cellSize

					svg.WriteString(fmt.Sprintf(
						`<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`,
						x,
						y,
						cellSize,
						cellSize,
						color,
					))
				}
			}

			bitIndex++
		}
	}

	svg.WriteString(`</svg>`)

	return svg.String()
}
