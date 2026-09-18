package radix

import (
	"testing"

	"github.com/codecrafters-io/redis-starter-go/app/structures/listpack"
)

func TestNew_InitialState(t *testing.T) {
	r := New(nil, nil)

	if r.HasValue {
		t.Errorf("HasValue = true, want false for a value-less node")
	}
	if len(r.Children) != 0 {
		t.Errorf("len(Children) = %d, want 0", len(r.Children))
	}
}

func TestInsert_SingleKey(t *testing.T) {
	r := New(nil, nil)
	lp := listpack.New()

	r.Insert([]byte("car"), lp)

	child := r.Children['c']
	if child == nil {
		t.Fatalf("Children['c'] = nil, want a node for \"car\"")
	}
	if string(child.Prefix) != "car" {
		t.Errorf("Prefix = %q, want %q", child.Prefix, "car")
	}
	if !child.HasValue || child.Value != lp {
		t.Errorf("HasValue/Value not set to the inserted listpack")
	}
}

func TestInsert_DivergingKeys_Split(t *testing.T) {
	// "car" then "cat" share "ca" and diverge at the third byte.
	r := New(nil, nil)
	lpCar := listpack.New()
	lpCat := listpack.New()

	r.Insert([]byte("car"), lpCar)
	r.Insert([]byte("cat"), lpCat)

	branch := r.Children['c']
	if branch == nil {
		t.Fatalf("Children['c'] = nil, want the shared-prefix branch node")
	}
	if string(branch.Prefix) != "ca" {
		t.Errorf("branch.Prefix = %q, want %q", branch.Prefix, "ca")
	}
	if branch.HasValue {
		t.Errorf("branch.HasValue = true, want false: \"ca\" was never inserted as its own key")
	}

	carNode := branch.Children['r']
	if carNode == nil {
		t.Fatalf("branch.Children['r'] = nil, want the \"car\" leaf")
	}
	if string(carNode.Prefix) != "r" || carNode.Value != lpCar {
		t.Errorf("carNode = {Prefix:%q, Value match:%v}, want {Prefix:\"r\", Value:lpCar}", carNode.Prefix, carNode.Value == lpCar)
	}

	catNode := branch.Children['t']
	if catNode == nil {
		t.Fatalf("branch.Children['t'] = nil, want the \"cat\" leaf")
	}
	if string(catNode.Prefix) != "t" || catNode.Value != lpCat {
		t.Errorf("catNode = {Prefix:%q, Value match:%v}, want {Prefix:\"t\", Value:lpCat}", catNode.Prefix, catNode.Value == lpCat)
	}
}

func TestInsert_ShorterKeyIsPrefixOfExisting(t *testing.T) {
	// "care" is inserted first, then "car" — a strict prefix of it.
	// The existing node's leftover suffix ("e") must survive the split.
	r := New(nil, nil)
	lpCare := listpack.New()
	lpCar := listpack.New()

	r.Insert([]byte("care"), lpCare)
	r.Insert([]byte("car"), lpCar)

	carNode := r.Children['c']
	if carNode == nil {
		t.Fatalf("Children['c'] = nil, want the \"car\" node")
	}
	if string(carNode.Prefix) != "car" {
		t.Errorf("carNode.Prefix = %q, want %q", carNode.Prefix, "car")
	}
	if !carNode.HasValue || carNode.Value != lpCar {
		t.Errorf("carNode HasValue/Value not set to lpCar")
	}

	careNode := carNode.Children['e']
	if careNode == nil {
		t.Fatalf("carNode.Children['e'] = nil, want the surviving \"care\" leaf")
	}
	if string(careNode.Prefix) != "e" {
		t.Errorf("careNode.Prefix = %q, want %q", careNode.Prefix, "e")
	}
	if !careNode.HasValue || careNode.Value != lpCare {
		t.Errorf("careNode HasValue/Value not set to lpCare")
	}
}

func TestInsert_ExactDuplicateUpdatesValueInPlace(t *testing.T) {
	r := New(nil, nil)
	lpOld := listpack.New()
	lpNew := listpack.New()

	r.Insert([]byte("car"), lpOld)
	r.Insert([]byte("car"), lpNew)

	if len(r.Children) != 1 {
		t.Errorf("len(Children) = %d, want 1: a duplicate key must not create a second node", len(r.Children))
	}

	child := r.Children['c']
	if child == nil || child.Value != lpNew {
		t.Errorf("duplicate insert did not overwrite the value in place")
	}
}

func TestInsert_FourKeys_MatchesWorkedExample(t *testing.T) {
	// Mirrors the walkthrough in Redis Internals/Radix Trees.md, section 6.
	r := New(nil, nil)
	lpCar := listpack.New()
	lpCat := listpack.New()
	lpDog := listpack.New()
	lpCare := listpack.New()

	r.Insert([]byte("car"), lpCar)
	r.Insert([]byte("cat"), lpCat)
	r.Insert([]byte("dog"), lpDog)
	r.Insert([]byte("care"), lpCare)

	dogNode := r.Children['d']
	if dogNode == nil || string(dogNode.Prefix) != "dog" || dogNode.Value != lpDog {
		t.Errorf("dogNode not attached correctly as a sibling of the 'c' branch")
	}

	branch := r.Children['c']
	if branch == nil || string(branch.Prefix) != "ca" {
		t.Fatalf("branch (\"ca\") missing or wrong Prefix: %+v", branch)
	}

	carNode := branch.Children['r']
	if carNode == nil || string(carNode.Prefix) != "r" || carNode.Value != lpCar {
		t.Fatalf("carNode wrong: %+v", carNode)
	}

	careNode := carNode.Children['e']
	if careNode == nil || string(careNode.Prefix) != "e" || careNode.Value != lpCare {
		t.Errorf("careNode wrong: %+v", careNode)
	}

	catNode := branch.Children['t']
	if catNode == nil || string(catNode.Prefix) != "t" || catNode.Value != lpCat {
		t.Errorf("catNode wrong: %+v", catNode)
	}
}
