package tagimport

import (
	"fmt"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/s7"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/xlsx"
)

func find(r *Result, name string) *Tag {
	for i := range r.Tags {
		if r.Tags[i].Name == name {
			return &r.Tags[i]
		}
	}
	return nil
}

func want(t *testing.T, r *Result, name, addr, typ string) *Tag {
	t.Helper()
	g := find(r, name)
	if g == nil {
		t.Fatalf("%s: not proposed (tags: %d, skipped: %+v)", name, len(r.Tags), r.Skipped)
	}
	if g.Address != addr || g.Type != typ {
		t.Fatalf("%s: got %s %s, want %s %s", name, g.Address, g.Type, addr, typ)
	}
	if !strings.Contains(g.Address, "{db}") {
		if _, err := s7.ParseAddress(g.Address, g.Type); err != nil {
			t.Fatalf("%s: the connector refuses %s %s: %v", name, g.Address, g.Type, err)
		}
	}
	return g
}

func skipped(r *Result, name string) string {
	for _, s := range r.Skipped {
		if s.Name == name {
			return s.Reason
		}
	}
	return ""
}

// The STEP 7 V5 source of a block with standard access; the offsets in the comments are those the DB editor shows.
const awl = `DATA_BLOCK DB 5
TITLE =Pumping station
VERSION : 0.1

  STRUCT
   Run : BOOL ;	//0.0 pump runs
   Fault : BOOL ;	//0.1
   Mode : BYTE ;	//1.0
   Speed : INT  := 1450;	//2.0 rpm
   Flags : ARRAY  [0 .. 9 ] OF BOOL ;	//4.0 .. 5.1
   Level : REAL  := 1.500000e+000;	//6.0 m
   Name : STRING  [5 ];	//10.0 (7 bytes, the next variable starts at 18)
   Hours : DINT ;	//18.0
   Motor : STRUCT
    Current : REAL ;	//22.0 A
    On : BOOL ;	//26.0
    Trip : BOOL ;	//26.1
   END_STRUCT ;
   Count : WORD ;	//28.0
   Temps : ARRAY  [1 .. 3 ] OF INT ;	//30.0 32.0 34.0
   Small : SINT ;	//36.0
   Runtime : TIME ;	//38.0
   Started : DATE_AND_TIME ;	//42.0
   Total : DWORD ;	//50.0
  END_STRUCT ;
BEGIN
   Run := FALSE;
   Speed := 1450;
   Name := 'P1';
END_DATA_BLOCK
`

func TestAWLLayout(t *testing.T) {
	r, err := Parse([]byte(awl), Options{Filename: "db5.awl"})
	if err != nil {
		t.Fatal(err)
	}
	want(t, r, "Run", "DB5.DBX0.0", "bool")
	want(t, r, "Fault", "DB5.DBX0.1", "bool")
	want(t, r, "Mode", "DB5.DBB1", "byte")
	want(t, r, "Speed", "DB5.DBW2", "int")
	want(t, r, "Flags[0]", "DB5.DBX4.0", "bool")
	want(t, r, "Flags[7]", "DB5.DBX4.7", "bool")
	want(t, r, "Flags[9]", "DB5.DBX5.1", "bool")
	want(t, r, "Level", "DB5.DBD6", "real")
	want(t, r, "Name", "DB5.DBS10.5", "string")
	want(t, r, "Hours", "DB5.DBD18", "dint")
	want(t, r, "Motor.Current", "DB5.DBD22", "real")
	want(t, r, "Motor.On", "DB5.DBX26.0", "bool")
	want(t, r, "Motor.Trip", "DB5.DBX26.1", "bool")
	want(t, r, "Count", "DB5.DBW28", "word")
	want(t, r, "Temps[1]", "DB5.DBW30", "int")
	want(t, r, "Temps[3]", "DB5.DBW34", "int")
	want(t, r, "Small", "DB5.DBB36", "sint")
	want(t, r, "Runtime", "DB5.DBD38", "dint")
	want(t, r, "Total", "DB5.DBD50", "dword")
	if g := find(r, "Run"); g.Comment != "0.0 pump runs" || g.Key != "run" || g.Block != "DB5" || g.NeedsDB {
		t.Fatalf("Run: %+v", g)
	}
	if find(r, "Motor.Current").Key != "motor_current" || find(r, "Temps[3]").Key != "temps_3" {
		t.Fatalf("keys: %s %s", find(r, "Motor.Current").Key, find(r, "Temps[3]").Key)
	}
	if !strings.Contains(skipped(r, "Started"), "not supported") {
		t.Fatalf("DATE_AND_TIME must be skipped: %+v", r.Skipped)
	}
	if len(r.Blocks) != 1 || r.Blocks[0].Number != 5 || r.Blocks[0].Size != 54 || r.Blocks[0].Tags != len(r.Tags) {
		t.Fatalf("block: %+v", r.Blocks)
	}
	if len(r.Warnings) == 0 {
		t.Fatal("a data block import must warn to compare the offsets")
	}
}

const tiaDB = `TYPE "udtValve"
VERSION : 0.1
   STRUCT
      Open { ExternalWritable := 'False'} : Bool;   // open command
      Closed : Bool;
      Position : Real;   // %
   END_STRUCT;

END_TYPE

DATA_BLOCK "DB_Čerpadlá"
{ S7_Optimized_Access := 'FALSE' }
VERSION : 0.1
NON_RETAIN
   STRUCT
      Tlak_výstup : Real;   // bar
      V1 : "udtValve";
      V : Array[1..2] of "udtValve";
      Matrix : Array[0..1, 0..2] of Int;
      Big : LReal;
      Txt : String[20];
      W : WString[10];
      Last : Bool;
   END_STRUCT;

BEGIN
   Tlak_výstup := 2.5;

END_DATA_BLOCK

DATA_BLOCK "DB_Optimized"
{ S7_Optimized_Access := 'TRUE' }
VERSION : 0.1
   VAR
      A : Int;
   END_VAR
BEGIN
END_DATA_BLOCK
`

func TestTIASourceWithUDT(t *testing.T) {
	r, err := Parse([]byte(tiaDB), Options{Filename: "pumps.db", DB: 12, Prefix: "p_", Existing: []string{"p_tlak_vystup"}})
	if err != nil {
		t.Fatal(err)
	}
	g := want(t, r, "Tlak_výstup", "DB12.DBD0", "real")
	if g.Key != "p_tlak_vystup_2" || g.Comment != "bar" || g.Block != "DB_Čerpadlá" {
		t.Fatalf("first tag: %+v", g)
	}
	want(t, r, "V1.Open", "DB12.DBX4.0", "bool")
	want(t, r, "V1.Closed", "DB12.DBX4.1", "bool")
	want(t, r, "V1.Position", "DB12.DBD6", "real")
	want(t, r, "V[1].Open", "DB12.DBX10.0", "bool")
	want(t, r, "V[1].Position", "DB12.DBD12", "real")
	want(t, r, "V[2].Open", "DB12.DBX16.0", "bool")
	want(t, r, "V[2].Position", "DB12.DBD18", "real")
	want(t, r, "Matrix[0,0]", "DB12.DBW22", "int")
	want(t, r, "Matrix[1,2]", "DB12.DBW32", "int")
	want(t, r, "Big", "DB12,LREAL34", "lreal")
	want(t, r, "Txt", "DB12.DBS42.20", "string")
	want(t, r, "Last", "DB12.DBX88.0", "bool") // 42 + 22 = 64, WString[10] = 24 bytes → 88
	if find(r, "V1.Open").Comment != "open command" {
		t.Fatalf("comment of a UDT member: %q", find(r, "V1.Open").Comment)
	}
	if !strings.Contains(skipped(r, "W"), "WString") {
		t.Fatalf("WString must be skipped: %+v", r.Skipped)
	}
	if !strings.Contains(skipped(r, "DB_Optimized"), "optimized") {
		t.Fatalf("the optimized block must be refused: %+v", r.Skipped)
	}
	if len(r.Blocks) != 2 || r.Blocks[0].Number != 12 || r.Blocks[1].Tags != 0 {
		t.Fatalf("blocks: %+v", r.Blocks)
	}
	for _, x := range r.Tags {
		if !strings.HasPrefix(x.Key, "p_") || len(x.Key) > 32 {
			t.Fatalf("key %q", x.Key)
		}
	}

	// without a block number the address keeps the placeholder
	r, err = Parse([]byte(tiaDB), Options{Filename: "pumps.db"})
	if err != nil {
		t.Fatal(err)
	}
	if g := find(r, "V1.Position"); g.Address != "DB{db}.DBD6" || !g.NeedsDB {
		t.Fatalf("placeholder: %+v", g)
	}
}

func TestMissingUDTStopsTheBlock(t *testing.T) {
	src := "DATA_BLOCK \"X\"\n{ S7_Optimized_Access := 'FALSE' }\nSTRUCT\n A : Int;\n M : \"udtMissing\";\n B : Int;\nEND_STRUCT;\nBEGIN\nEND_DATA_BLOCK\n"
	r, err := Parse([]byte(src), Options{DB: 1})
	if err != nil {
		t.Fatal(err)
	}
	want(t, r, "A", "DB1.DBW0", "int")
	if find(r, "B") != nil {
		t.Fatal("variables after an unknown data type must not get an address")
	}
	if !strings.Contains(skipped(r, "M"), "udtMissing") || r.Blocks[0].Note == "" {
		t.Fatalf("skipped: %+v, block: %+v", r.Skipped, r.Blocks)
	}
	// an instance data block of a function block has no declaration in its source
	r, err = Parse([]byte("DATA_BLOCK DB 7\nFB 1\nBEGIN\nEND_DATA_BLOCK\n"), Options{})
	if err != nil || len(r.Tags) != 0 || !strings.Contains(r.Blocks[0].Note, "instance") {
		t.Fatalf("instance block: %v %+v", err, r)
	}
}

func TestSymbolTables(t *testing.T) {
	sdf := "\"Start_PB                \",\"E       0.0\",\"BOOL      \",\"start push button\"\r\n" +
		"\"Motor_On                \",\"A       4.1\",\"BOOL      \",\"\"\r\n" +
		"\"Tlak čerpadla           \",\"MD     20\",\"REAL      \",\"bar\"\r\n" +
		"\"Otacky                  \",\"MW     10\",\"INT       \",\"\"\r\n" +
		"\"Doba                    \",\"MD     30\",\"TIME      \",\"\"\r\n" +
		"\"Analog_In               \",\"PEW   256\",\"WORD      \",\"\"\r\n" +
		"\"DB_Data                 \",\"DB      5\",\"DB      5\",\"\"\r\n" +
		"\"Timer1                  \",\"T       1\",\"TIMER     \",\"\"\r\n" +
		"\"Odd                     \",\"MW     11\",\"REAL      \",\"\"\r\n"
	// Windows-1250 bytes, as STEP 7 writes them on a Slovak system
	enc := strings.NewReplacer("č", "\xe8").Replace(sdf)
	r, err := Parse([]byte(enc), Options{Filename: "symbols.sdf", Encoding: "windows-1250"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r.Format, "SDF") {
		t.Fatalf("format %q", r.Format)
	}
	want(t, r, "Start_PB", "I0.0", "bool")
	want(t, r, "Motor_On", "Q4.1", "bool")
	if g := want(t, r, "Tlak čerpadla", "MD20", "real"); g.Key != "tlak_cerpadla" || g.Comment != "bar" || g.Area != "m" {
		t.Fatalf("%+v", g)
	}
	want(t, r, "Otacky", "MW10", "int")
	want(t, r, "Doba", "MD30", "dint")
	for name, part := range map[string]string{"Analog_In": "peripheral", "DB_Data": "block", "Timer1": "block", "Odd": "needs"} {
		if !strings.Contains(skipped(r, name), part) {
			t.Fatalf("%s: reason %q", name, skipped(r, name))
		}
	}

	asc := fmt.Sprintf("126,%-24s%-12s%-10s%s\r\n126,%-24s%-12s%-10s%s\r\n", "Start_PB", "E       0.0", "BOOL", "start push button", "Level", "MD     40", "REAL", "")
	r, err = Parse([]byte(asc), Options{Filename: "symbols.asc"})
	if err != nil {
		t.Fatal(err)
	}
	want(t, r, "Start_PB", "I0.0", "bool")
	if g := want(t, r, "Level", "MD40", "real"); g.Comment != "" {
		t.Fatalf("%+v", g)
	}
	if find(r, "Start_PB").Comment != "start push button" {
		t.Fatalf("comment %q", find(r, "Start_PB").Comment)
	}

	csvText := "Name;Address;Data Type;Comment\nLevel;%MD100;Real;m\nPump;%Q0.0;Bool;\nBad;%MX;Bool;\n"
	r, err = Parse([]byte(csvText), Options{Filename: "tags.csv"})
	if err != nil {
		t.Fatal(err)
	}
	want(t, r, "Level", "MD100", "real")
	want(t, r, "Pump", "Q0.0", "bool")
	if skipped(r, "Bad") == "" {
		t.Fatal("an invalid address must be reported")
	}
}

func TestTIATagTableExcel(t *testing.T) {
	rows := func(rs ...[]string) [][]xlsx.Cell {
		var out [][]xlsx.Cell
		for _, r := range rs {
			var row []xlsx.Cell
			for _, c := range r {
				row = append(row, xlsx.T(c))
			}
			out = append(out, row)
		}
		return out
	}
	wb, err := xlsx.Bytes(
		xlsx.Sheet{Name: "PLC Tags", Rows: rows(
			[]string{"Name", "Path", "Data Type", "Logical Address", "Comment", "Hmi Visible", "Hmi Accessible", "Hmi Writeable", "Typeobject ID", "Version ID"},
			[]string{"Start", "Default tag table", "Bool", "%I0.0", "start button", "True", "True", "True", "", ""},
			[]string{"Hladina_nádrž", "Default tag table", "Real", "%MD20", "m", "True", "True", "True", "", ""},
			[]string{"Counter1", "Default tag table", "DInt", "%MD24", "", "True", "True", "True", "", ""},
			[]string{"Clock_1Hz", "Default tag table", "Bool", "%M0.5", "", "True", "True", "True", "", ""},
			[]string{"AI_raw", "Default tag table", "Word", "%IW64:P", "", "True", "True", "True", "", ""},
			[]string{"Stamp", "Default tag table", "DTL", "%MB100", "", "True", "True", "True", "", ""},
		)},
		xlsx.Sheet{Name: "User Constants", Rows: rows([]string{"Name", "Path", "Data Type", "Value", "Comment"}, []string{"MAX", "Default tag table", "Int", "10", ""})},
	)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(wb, Options{Filename: "PLCTags.xlsx"})
	if err != nil {
		t.Fatal(err)
	}
	want(t, r, "Start", "I0.0", "bool")
	if g := want(t, r, "Hladina_nádrž", "MD20", "real"); g.Key != "hladina_nadrz" || g.Comment != "m" {
		t.Fatalf("%+v", g)
	}
	want(t, r, "Counter1", "MD24", "dint")
	want(t, r, "Clock_1Hz", "M0.5", "bool")
	if !strings.Contains(skipped(r, "AI_raw"), "peripheral") || !strings.Contains(skipped(r, "Stamp"), "DTL") {
		t.Fatalf("skipped: %+v", r.Skipped)
	}
	if find(r, "MAX") != nil || len(r.Tags) != 4 {
		t.Fatalf("constants must not become tags: %d", len(r.Tags))
	}
}

const tagXML = `<?xml version="1.0" encoding="utf-8"?>
<Document><SW.Tags.PlcTagTable ID="0"><AttributeList><Name>Default tag table</Name></AttributeList><ObjectList>
<SW.Tags.PlcTag ID="1" CompositionName="Tags"><AttributeList><DataTypeName>Bool</DataTypeName><ExternalAccessible>true</ExternalAccessible><LogicalAddress>%Q0.3</LogicalAddress><Name>Valve_Open</Name></AttributeList>
 <ObjectList><MultilingualText ID="2" CompositionName="Comment"><ObjectList><MultilingualTextItem ID="3" CompositionName="Items"><AttributeList><Culture>en-US</Culture><Text>opens the valve</Text></AttributeList></MultilingualTextItem></ObjectList></MultilingualText></ObjectList></SW.Tags.PlcTag>
<SW.Tags.PlcTag ID="4" CompositionName="Tags"><AttributeList><DataTypeName>Int</DataTypeName><LogicalAddress>%MW12</LogicalAddress><Name>Speed</Name></AttributeList></SW.Tags.PlcTag>
</ObjectList></SW.Tags.PlcTagTable></Document>`

const dbXML = `<?xml version="1.0" encoding="utf-8"?>
<Document><SW.Blocks.GlobalDB ID="0"><AttributeList>
<Interface><Sections xmlns="http://www.siemens.com/automation/Openness/SW/Interface/v5"><Section Name="Static">
 <Member Name="Level" Datatype="Real"><Comment><MultiLanguageText Lang="en-US">tank level</MultiLanguageText></Comment></Member>
 <Member Name="Alarm" Datatype="Bool" />
 <Member Name="Pump" Datatype="Struct"><Member Name="Run" Datatype="Bool" /><Member Name="Hours" Datatype="DInt" /></Member>
 <Member Name="Setpoints" Datatype="Array[0..2] of Int" />
 <Member Name="V" Datatype="&quot;udtValve&quot;"><Sections><Section Name="None"><Member Name="Open" Datatype="Bool" /><Member Name="Pos" Datatype="Real" /></Section></Sections></Member>
</Section></Sections></Interface>
<MemoryLayout>Standard</MemoryLayout><Name>DB_Tank</Name><Number>20</Number><ProgrammingLanguage>DB</ProgrammingLanguage>
</AttributeList></SW.Blocks.GlobalDB></Document>`

func TestOpennessXML(t *testing.T) {
	r, err := Parse([]byte(tagXML), Options{Filename: "tags.xml"})
	if err != nil {
		t.Fatal(err)
	}
	if g := want(t, r, "Valve_Open", "Q0.3", "bool"); g.Comment != "opens the valve" {
		t.Fatalf("%+v", g)
	}
	want(t, r, "Speed", "MW12", "int")

	r, err = Parse([]byte(dbXML), Options{Filename: "DB_Tank.xml"})
	if err != nil {
		t.Fatal(err)
	}
	if g := want(t, r, "Level", "DB20.DBD0", "real"); g.Comment != "tank level" {
		t.Fatalf("%+v", g)
	}
	want(t, r, "Alarm", "DB20.DBX4.0", "bool")
	want(t, r, "Pump.Run", "DB20.DBX6.0", "bool")
	want(t, r, "Pump.Hours", "DB20.DBD8", "dint")
	want(t, r, "Setpoints[0]", "DB20.DBW12", "int")
	want(t, r, "Setpoints[2]", "DB20.DBW16", "int")
	want(t, r, "V.Open", "DB20.DBX18.0", "bool")
	want(t, r, "V.Pos", "DB20.DBD20", "real")
	if r.Blocks[0].Number != 20 || r.Blocks[0].Size != 24 {
		t.Fatalf("block %+v", r.Blocks[0])
	}
	opt := strings.Replace(dbXML, "<MemoryLayout>Standard", "<MemoryLayout>Optimized", 1)
	r, err = Parse([]byte(opt), Options{})
	if err != nil || len(r.Tags) != 0 || !strings.Contains(r.Blocks[0].Note, "optimized") {
		t.Fatalf("optimized: %v %+v", err, r)
	}
}

func TestKeysAndLimits(t *testing.T) {
	for name, wantKey := range map[string]string{
		"Tank_Level": "tank_level", "Čerpadlo Č.1 – prúd": "cerpadlo_c_1_prud", "1st value": "t_1st_value", "___": "tag", "Straße": "strasse",
		"A_very_long_symbol_name_of_a_signal_in_the_plant": "a_very_long_sym_nal_in_the_plant",
	} {
		if k := MakeKey("", name); k != wantKey || len(k) > 32 {
			t.Fatalf("%q → %q, want %q", name, k, wantKey)
		}
	}
	if k := MakeKey("m1_", "1st"); k != "m1_1st" {
		t.Fatalf("prefix: %q", k)
	}
	big := "DATA_BLOCK DB 1\nSTRUCT\n A : ARRAY [0 .. 4999] OF INT ;\n After : INT ;\nEND_STRUCT ;\nBEGIN\nEND_DATA_BLOCK\n"
	r, err := Parse([]byte(big), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Tags) != maxArrayItems+1 {
		t.Fatalf("tags %d", len(r.Tags))
	}
	want(t, r, "After", "DB1.DBW10000", "int") // the elements that are not proposed still take their place
	for _, bad := range []string{"", "hello world", "DATA_BLOCK DB 1\nSTRUCT\n A : INT\n"} {
		if _, err := Parse([]byte(bad), Options{}); err == nil {
			t.Fatalf("%q must be refused", bad)
		}
	}
	if _, err := Parse([]byte(awl), Options{Prefix: "Bad Prefix"}); err == nil {
		t.Fatal("an invalid prefix must be refused")
	}
}
