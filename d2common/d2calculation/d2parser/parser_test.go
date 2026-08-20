package d2parser

import (
	"math"
	"testing"
)

func TestEmptyInput(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"", 0},
		{" ", 0},
		{"\t\t \t\t \t", 0},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestConstantExpression(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"0", 0},
		{"5", 5},
		{"455", 455},
		{"789", 789},
		{"3242", 3242},
		{"45454", 45454},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestUnaryOperations(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"+0", 0},
		{"-0", 0},
		{"+455", 455},
		{"++455", 455},
		{"+++455", 455},
		{"-455", -455},
		{"--455", 455},
		{"---455", -455},
		{"+-+789", -789},
		{"-++3242", -3242},
		{"++--+-+45454", -45454},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestArithmeticBinaryOperations(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"1 + 2", 3},
		{"54+56", 54 + 56},
		{"9212-2121", 9212 - 2121},
		{"1+2-5", -2},
		{"5-3-1", 1},
		{"4*5", 20},
		{"512/2", 256},
		{"10/9", 1},
		{"1/3*5", 0},
		{"2^3^2", int(math.Pow(2., 9.))},
		{"4^2*2+1", 33},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestParentheses(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"(1+2)*5", 15},
		{"(99-98)/2", 0},
		{"((3+2)*6)/3", 10},
		{"(3+2)*(6/3)", 10},
		{"(20+10)/6/5", 1},
		{"(20+10)/(6/5)", 30},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestLackFinalParethesis(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"(3+2)*(6/3", 10},
		{"(20+10)/(6/5", 30},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestLogicalBinaryOperations(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result bool
	}{
		{"1 < 2", true},
		{"1 < -5", false},
		{"1 <= 5", true},
		{"1 <= 10", true},
		{"5 <= 1", false},
		{"1 <= 1", true},
		{"5 > 10", false},
		{"54 >= 100", false},
		{"45 >= 45", true},
		{"10 == 10", true},
		{"10 == 1", false},
		{"10 != 1", true},
		{"10 != 10", false},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if (res == 0 && row.result) || (res != 0 && !row.result) {
			t.Errorf("Expression %v gave wrong result, got %d, want %v", row.expr, res, row.result)
		}
	}
}

func TestLogicalAndArithmetic(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result bool
	}{
		{"(1 < 2)*(5 < 10)", true},
		{"(1 < -5)+(1 == 5)", false},
		{"(5 > 10)*(10 == 10)", false},
		{"(45 >= 45)+(1 > 500000)", true},
		{"(10 == 10)*(30 > 50)+(5 >= 5)", true},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if (res == 0 && row.result) || (res != 0 && !row.result) {
			t.Errorf("Expression %v gave wrong result, got %d, want %v", row.expr, res, row.result)
		}
	}
}

func TestTernaryOperator(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"5 > 1 ? 3 : 5", 3},
		{"5 <= 1 ? 3 : 5", 5},
		{"(1 < 10)*(5 < 3) ? 43 : 5 == 5 ? 1 : 2", 1},
		{"(1 < 10)*(5 < 3) ? 43 : 5 != 5 ? 1 : 2", 2},
		{"(1 < 10)*(5 > 3) ? 43 : 5 == 5 ? 1 : 2", 43},
		{"(1 < 10)*(5 > 3) ? 43 != 0 ? 65 : 32 : 5 == 5 ? 1 : 2", 65},
		{"(1 < 10)*(5 > 3) ? 43 == 0 ? 65 : 32 : 5 == 5 ? 1 : 2", 32},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestBuiltinFunctions(t *testing.T) {
	parser := New()

	table := []struct {
		expr   string
		result int
	}{
		{"min(5, 2)", 2},
		{"min(4^6, 5+10)", 15},
		{"max(10, 4*3)", 12},
		{"max(50-2, 50-3)", 48},
	}

	for _, row := range table {
		c := parser.Parse(row.expr)
		res := c.Eval()

		if res != row.result {
			t.Errorf("Expression %v gave wrong result, got %d, want %d", row.expr, res, row.result)
		}
	}
}

func TestRandFunction(t *testing.T) {
	parser := New()
	c := parser.Parse("rand(1,5)")

	// rand(v1,v2) is implemented as a coin flip between its two operands.
	// The old test re-seeded the global math/rand source and compared two
	// sequences, but top-level rand.Seed is a no-op for go >= 1.24 (see the
	// Go 1.24 release notes), so assert the contract instead of a fixed
	// sequence: every result is one of the operands, and both sides occur.
	seen := map[int]int{}

	for i := 0; i < 1000; i++ {
		v := c.Eval()
		if v != 1 && v != 5 {
			t.Fatalf("rand(1,5) returned %d; want 1 or 5", v)
		}

		seen[v]++
	}

	if len(seen) != 2 {
		t.Errorf("rand(1,5) returned only %v over 1000 evals; want both 1 and 5", seen)
	}
}

func BenchmarkSimpleExpression(b *testing.B) {
	parser := New()
	expr := "(1 < 10)*(5 > 3) ? 43 == 0 ? 65 : 32 : 5 == 5 ? 1 : 2"

	for n := 0; n < b.N; n++ {
		parser.Parse(expr)
	}
}
