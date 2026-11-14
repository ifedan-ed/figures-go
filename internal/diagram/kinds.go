// Package diagram draws the diagram figures: flowcharts, timelines,
// cycles, comparison tables, concept maps, Venn diagrams, pathways and
// schematics. Each is laid out here from a checked spec, so the model
// names the parts and their links and never places anything itself
// (except on a schematic's coarse grid, which is checked).
package diagram

import "github.com/ifedan-ed/figures-go/internal/core"

// Kinds is this package's figure types.
func Kinds() []core.Kind {
	return []core.Kind{flowchartKind, pathwayKind, timelineKind, cycleKind, tableKind, vennKind, conceptKind, schematicKind}
}
