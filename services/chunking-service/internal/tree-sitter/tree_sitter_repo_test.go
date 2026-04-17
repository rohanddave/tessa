package treesitter

import (
	"testing"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
)

func TestExtractFileMetadataCPP(t *testing.T) {
	parser := NewTreeSitterRepo()
	source := `
#include <vector>
#include "billing/invoice.hpp"

namespace billing {

class InvoiceService {
public:
  int createInvoice(int customerId);
};

int InvoiceService::createInvoice(int customerId) {
  return customerId;
}

int calculateTax(int amount) {
  return amount;
}

}
`

	result, err := parser.ExtractFileMetadata(source, "cpp")
	if err != nil {
		t.Fatalf("extract C++ metadata: %v", err)
	}

	if len(result.Imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(result.Imports))
	}
	if !hasDeclaration(result.Classes, "InvoiceService") {
		t.Fatalf("expected class InvoiceService in %+v", result.Classes)
	}
	if !hasDeclaration(result.Methods, "createInvoice") {
		t.Fatalf("expected method createInvoice in %+v", result.Methods)
	}
	if !hasDeclaration(result.Functions, "calculateTax") {
		t.Fatalf("expected function calculateTax in %+v", result.Functions)
	}
	if !hasDeclaration(result.ModuleLevelDeclarations, "billing") {
		t.Fatalf("expected namespace billing module declaration in %+v", result.ModuleLevelDeclarations)
	}
}

func hasDeclaration(declarations []ports.CodeDeclaration, name string) bool {
	for _, declaration := range declarations {
		if declaration.Name == name {
			return true
		}
	}
	return false
}
