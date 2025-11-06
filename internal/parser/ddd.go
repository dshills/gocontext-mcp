package parser

import (
	"strings"

	"github.com/dshills/gocontext-mcp/pkg/types"
)

// detectDDDPatterns identifies domain-driven design patterns based on naming conventions
func detectDDDPatterns(sym *types.Symbol) {
	// Only apply DDD detection to types, interfaces, and structs
	if sym.Kind != types.KindStruct && sym.Kind != types.KindInterface && sym.Kind != types.KindType {
		return
	}

	// Check all patterns
	checkAggregateRoot(sym)
	checkEntity(sym)
	checkValueObject(sym)
	checkRepository(sym)
	checkService(sym)
	checkCommand(sym)
	checkQuery(sym)
	checkHandler(sym)
}

func checkAggregateRoot(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "Aggregate") || strings.HasSuffix(sym.Name, "AggregateRoot") {
		sym.IsAggregateRoot = true
		sym.IsEntity = true // Aggregates are also entities
	}
}

func checkEntity(sym *types.Symbol) {
	if sym.IsEntity {
		return // Already marked as entity
	}

	if strings.HasSuffix(sym.Name, "Entity") {
		sym.IsEntity = true
		return
	}

	// Additional patterns for Entity detection (types with common entity-like names)
	entityIndicators := []string{"Order", "User", "Product", "Account", "Customer", "Item"}
	for _, indicator := range entityIndicators {
		if strings.Contains(sym.Name, indicator) && !strings.HasSuffix(sym.Name, "Service") &&
			!strings.HasSuffix(sym.Name, "Repository") && !strings.HasSuffix(sym.Name, "Handler") {
			sym.IsEntity = true
			return
		}
	}
}

func checkValueObject(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "VO") || strings.HasSuffix(sym.Name, "ValueObject") {
		sym.IsValueObject = true
	}
}

func checkRepository(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "Repository") || strings.HasSuffix(sym.Name, "Repo") {
		sym.IsRepository = true
	}
}

func checkService(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "Service") {
		sym.IsService = true
	}
}

func checkCommand(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "Command") || strings.HasSuffix(sym.Name, "Cmd") {
		sym.IsCommand = true
	}
}

func checkQuery(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "Query") {
		sym.IsQuery = true
	}
}

func checkHandler(sym *types.Symbol) {
	if strings.HasSuffix(sym.Name, "Handler") {
		sym.IsHandler = true
	}
}

// IsEntityLikeStruct analyzes struct fields to determine if it's likely an entity
// This is used as a secondary check for entity detection
func IsEntityLikeStruct(fields []string) bool {
	// Check for common entity field patterns
	hasID := false

	for _, field := range fields {
		fieldLower := strings.ToLower(field)

		// Check for ID field
		if fieldLower == "id" || strings.HasSuffix(fieldLower, "id") {
			hasID = true
		}
	}

	// An entity typically has an ID field
	return hasID
}
