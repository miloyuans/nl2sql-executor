package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Catalog struct {
	Tables map[string]*Table
}

type Table struct {
	Schema  string
	Name    string
	Comment string
	Columns map[string]*Column
}

type Column struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Family  string `json:"family"`
	Comment string `json:"comment"`
}

type rawCatalog struct {
	Schemas []struct {
		Schema string `json:"schema"`
		Tables []struct {
			Table   string   `json:"table"`
			Comment string   `json:"comment"`
			Columns []Column `json:"columns"`
		} `json:"tables"`
	} `json:"schemas"`
}

func NewEmptyCatalog() *Catalog { return &Catalog{Tables: map[string]*Table{}} }

func LoadCatalog(path string) (*Catalog, error) {
	if strings.TrimSpace(path) == "" {
		return NewEmptyCatalog(), fmt.Errorf("empty catalog path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw rawCatalog
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	c := NewEmptyCatalog()
	for _, s := range raw.Schemas {
		for _, t := range s.Tables {
			ft := NormalizeFullName(s.Schema, t.Table)
			tbl := &Table{Schema: s.Schema, Name: t.Table, Comment: t.Comment, Columns: map[string]*Column{}}
			for i := range t.Columns {
				col := t.Columns[i]
				tbl.Columns[strings.ToLower(col.Name)] = &col
			}
			c.Tables[ft] = tbl
		}
	}
	return c, nil
}

func NormalizeFullName(schema, table string) string {
	return strings.ToLower(strings.TrimSpace(schema) + "." + strings.TrimSpace(table))
}

func (c *Catalog) Empty() bool { return c == nil || len(c.Tables) == 0 }

func (c *Catalog) HasTable(schema, table string) bool {
	if c == nil || len(c.Tables) == 0 {
		return true
	}
	_, ok := c.Tables[NormalizeFullName(schema, table)]
	return ok
}

func (c *Catalog) HasColumn(schema, table, col string) bool {
	if c == nil || len(c.Tables) == 0 {
		return true
	}
	t, ok := c.Tables[NormalizeFullName(schema, table)]
	if !ok {
		return false
	}
	_, ok = t.Columns[strings.ToLower(col)]
	return ok
}
