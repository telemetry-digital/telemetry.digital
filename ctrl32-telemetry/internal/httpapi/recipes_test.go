package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

func testRecipe() recipeIn {
	return recipeIn{Name: " Mixing ", Elements: []recipeElement{
		{Key: "speed", Label: "Speed", Unit: "rpm", Min: f64(0), Max: f64(1500)},
		{Key: "temp", Type: "number", Decimals: 1, Min: f64(10), Max: f64(90)},
		{Key: "heating", Type: "bool", Min: f64(1)},
		{Key: "product", Type: "text"},
	}, Targets: []recipeTarget{
		{Name: "Mixer 1", DeviceID: "0198A000-0000-7000-8000-000000000001", Prefix: "m1_"},
		{Name: "Mixer 2", DeviceID: "0198a000-0000-7000-8000-000000000001", Prefix: "m2_"},
	}}
}

func TestCheckRecipe(t *testing.T) {
	in := testRecipe()
	if msg := checkRecipe(&in); msg != "" {
		t.Fatal(msg)
	}
	if in.Name != "Mixing" || in.Elements[0].Type != "number" || in.Elements[1].Label != "temp" {
		t.Fatalf("not normalized: %+v", in)
	}
	if in.Elements[2].Min != nil {
		t.Fatal("limits of a bool element must be dropped")
	}
	if in.Targets[0].ID != "u1" || in.Targets[1].ID != "u2" || in.Targets[0].DeviceID != "0198a000-0000-7000-8000-000000000001" {
		t.Fatalf("unit ids: %+v", in.Targets)
	}
	bad := map[string]func(*recipeIn){
		"name":               func(r *recipeIn) { r.Name = " " },
		"elements: 1":        func(r *recipeIn) { r.Elements = nil },
		"key must match":     func(r *recipeIn) { r.Elements[0].Key = "Speed" },
		"used twice":         func(r *recipeIn) { r.Elements[1].Key = "speed" },
		"type must be":       func(r *recipeIn) { r.Elements[0].Type = "int" },
		"decimals":           func(r *recipeIn) { r.Elements[0].Decimals = 7 },
		"min is above max":   func(r *recipeIn) { r.Elements[0].Min = f64(2000) },
		"choose a device":    func(r *recipeIn) { r.Targets[0].DeviceID = "x" },
		"key prefix":         func(r *recipeIn) { r.Targets[0].Prefix = "M1" },
		"longer than 32":     func(r *recipeIn) { r.Elements[0].Key = "a" + strings.Repeat("b", 30) },
		"listed twice":       func(r *recipeIn) { r.Targets[1].Prefix = "m1_" },
		"unit id must be":    func(r *recipeIn) { r.Targets[0].ID, r.Targets[1].ID = "a", "a" },
		"name 1 to 60":       func(r *recipeIn) { r.Targets[0].Name = "" },
		"description":        func(r *recipeIn) { r.Description = strings.Repeat("x", 501) },
		"limits must be fin": func(r *recipeIn) { r.Elements[0].Max = f64(inf()) },
	}
	for want, change := range bad {
		in := testRecipe()
		change(&in)
		if msg := checkRecipe(&in); !strings.Contains(msg, want) {
			t.Errorf("%s: got %q", want, msg)
		}
	}
	// an id given by the client is kept, a new unit gets a free one
	in = testRecipe()
	in.Targets[1].ID = "u1"
	if msg := checkRecipe(&in); msg != "" || in.Targets[0].ID == "u1" || in.Targets[1].ID != "u1" {
		t.Fatalf("free id: %q %+v", msg, in.Targets)
	}
}

func inf() float64 { var z float64; return 1 / z }

func TestCheckRecipeValues(t *testing.T) {
	in := testRecipe()
	_ = checkRecipe(&in)
	var vals map[string]any
	_ = json.Unmarshal([]byte(`{"speed": 1200, "temp": 45.26, "heating": true, "product": "A-12", "old": 5}`), &vals)
	out, msg := checkRecipeValues(in.Elements, vals)
	if msg != "" {
		t.Fatal(msg)
	}
	if out["temp"] != 45.3 || out["speed"] != 1200.0 || out["heating"] != true || out["product"] != "A-12" {
		t.Fatalf("values: %v", out)
	}
	if _, ok := out["old"]; ok {
		t.Fatal("a value without an element must be dropped")
	}
	for want, js := range map[string]string{
		"Speed: a value is needed":      `{"temp": 45, "heating": true, "product": ""}`,
		"above the maximum 1500":        `{"speed": 1500.6, "temp": 45, "heating": true, "product": ""}`,
		"below the minimum 10":          `{"speed": 1, "temp": 9.94, "heating": true, "product": ""}`,
		"Speed: a number is needed":     `{"speed": "1", "temp": 45, "heating": true, "product": ""}`,
		"heating: true or false":        `{"speed": 1, "temp": 45, "heating": 1, "product": ""}`,
		"product: text is needed":       `{"speed": 1, "temp": 45, "heating": true, "product": 5}`,
		"product: at most 254 bytes of": `{"speed": 1, "temp": 45, "heating": true, "product": "` + strings.Repeat("x", 255) + `"}`,
		"temp: a value is needed":       `{"speed": 1, "temp": null, "heating": true, "product": ""}`,
	} {
		var v map[string]any
		_ = json.Unmarshal([]byte(js), &v)
		if _, msg := checkRecipeValues(in.Elements, v); !strings.Contains(msg, want) {
			t.Errorf("%s: got %q", want, msg)
		}
	}
	// rounding happens before the limit: 9.96 with one decimal is 10.0 and passes
	_ = json.Unmarshal([]byte(`{"speed": 1, "temp": 9.96, "heating": false, "product": ""}`), &vals)
	if out, msg := checkRecipeValues(in.Elements, vals); msg != "" || out["temp"] != 10.0 {
		t.Fatalf("rounding: %q %v", msg, out)
	}
}

func TestRecipeSame(t *testing.T) {
	num := recipeElement{Type: "number", Decimals: 1}
	if !recipeSame(num, 45.3, float32(45.34)) || recipeSame(num, 45.3, 45.4) || recipeSame(num, 45.3, nil) || !recipeSame(recipeElement{Type: "number"}, 1200.0, int16(1200)) || recipeSame(num, 45.3, "45.3") {
		t.Fatal("number")
	}
	b := recipeElement{Type: "bool"}
	if !recipeSame(b, true, true) || recipeSame(b, true, false) || !recipeSame(b, false, uint8(0)) || recipeSame(b, true, nil) {
		t.Fatal("bool")
	}
	tx := recipeElement{Type: "text"}
	if !recipeSame(tx, "A-12", "A-12") || recipeSame(tx, "A-12", "A-13") || recipeSame(tx, "", nil) {
		t.Fatal("text")
	}
}

func TestRecipeCSV(t *testing.T) {
	in := testRecipe()
	_ = checkRecipe(&in)
	recs := []recipeRecord{
		{Number: 1, Name: "Cream, light", Note: "first", Values: map[string]any{"speed": 1200.0, "temp": 45.5, "heating": true, "product": "A-12"}},
		{Number: 7, Name: "Gel", Values: map[string]any{"speed": 300.0, "temp": 20.0, "heating": false, "product": ""}},
	}
	text := string(recipeCSV(in.Elements, recs))
	if !strings.HasPrefix(text, "\ufeffnumber,name,note,speed,temp,heating,product\n1,\"Cream, light\",first,1200,45.5,true,A-12\n7,Gel,,300,20,false,\n") {
		t.Fatalf("csv: %q", text)
	}
	back, err := parseRecipeCSV(in.Elements, text)
	if err != nil || len(back) != 2 || back[0].Name != "Cream, light" || back[0].Values["temp"] != 45.5 || back[1].Values["heating"] != false || back[1].Number != 7 {
		t.Fatalf("round trip: %v %+v", err, back)
	}
	// a spreadsheet with semicolons and decimal commas, lines in any order
	back, err = parseRecipeCSV(in.Elements, "Number;Name;speed;temp;heating;product\r\n9;Late;100;55,5;1;x\r\n2;Early;100;20;0;y\r\n\r\n")
	if err != nil || len(back) != 2 || back[0].Number != 2 || back[1].Values["temp"] != 55.5 || back[1].Values["heating"] != true {
		t.Fatalf("semicolons: %v %+v", err, back)
	}
	for want, text := range map[string]string{
		`column "temp" is missing`:     "number,name,speed,heating,product\n1,a,1,true,x\n",
		`column "name" is missing`:     "number,speed,temp,heating,product\n1,1,20,true,x\n",
		`is not an element`:            "number,name,speed,temp,heating,product,extra\n1,a,1,20,true,x,1\n",
		`line 2, speed`:                "number,name,speed,temp,heating,product\n1,a,fast,20,true,x\n",
		`line 3: number 1 is there`:    "number,name,speed,temp,heating,product\n1,a,1,20,true,x\n1,b,1,20,true,x\n",
		`line 2: Speed: 1501 is above`: "number,name,speed,temp,heating,product\n1,a,1501,20,true,x\n",
		`line 2, heating`:              "number,name,speed,temp,heating,product\n1,a,1,20,yes,x\n",
		`line 2: number must be`:       "number,name,speed,temp,heating,product\n0,a,1,20,true,x\n",
		`header line`:                  "number,name,speed,temp,heating,product\n",
		`there twice`:                  "number,name,name,speed,temp,heating,product\n1,a,a,1,20,true,x\n",
		`line 2: name`:                 "number,name,speed,temp,heating,product\n1,,1,20,true,x\n",
	} {
		if _, err := parseRecipeCSV(in.Elements, text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
}
