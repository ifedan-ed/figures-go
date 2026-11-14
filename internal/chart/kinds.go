// Package chart draws the chart, clinical chart and maths figure types:
// line, bar, scatter, area and box charts; growth, blood pressure, zone
// and dose–response charts; and function plots.
package chart

import "github.com/ifedan-ed/figures-go/internal/core"

// Kinds is this package's figure types.
func Kinds() []core.Kind {
	return []core.Kind{lineChart, barChart, scatterChart, areaChart, boxPlot, growthChart, bpChart, zoneChart, doseResponse, functionPlot}
}
