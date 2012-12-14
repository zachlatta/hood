package hood

import (
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type Hood struct {
	Db       *sql.DB
	Dialect  Dialect
	Qo       Qo // the query object
	selector string
	where    string
	params   []interface{}
	paramNum int
	limit    string
	offset   string
	orderBy  string
	joins    []string
	groupBy  string
	having   string
}

type Qo interface {
	Prepare(query string) (*sql.Stmt, error)
}

type (
	Id int64
	Pk struct {
		Name string
		Type reflect.Type
	}
	Model struct {
		Pk     *Pk
		Table  string
		Fields map[string]interface{}
	}
)

func New(database *sql.DB, dialect Dialect) *Hood {
	return &Hood{
		Db:      database,
		Dialect: dialect,
	}
}

func (hood *Hood) Begin() *Hood {
	c := new(Hood)
	*c = *hood
	q, err := hood.Db.Begin()
	if err != nil {
		panic(err)
	}
	c.Qo = q

	return c
}

func (hood *Hood) Commit() error {
	if v, ok := hood.Qo.(*sql.Tx); ok {
		return v.Commit()
	}
	return nil
}

func (hood *Hood) Select(selector string, table interface{}) *Hood {
	if selector == "" {
		selector = "*"
	}
	from := ""
	switch f := table.(type) {
	case string:
		from = f
	case interface{}:
		from = snakeCaseName(f)
	}
	if from == "" {
		panic("FROM cannot be empty")
	}
	hood.selector = fmt.Sprintf("SELECT %v FROM %v", selector, from)

	return hood
}

func (hood *Hood) Where(query interface{}, args ...interface{}) *Hood {
	where := ""
	switch typedQuery := query.(type) {
	case string: // custom query
		where = hood.substituteMarkers(typedQuery)
		hood.params = args
	case int: // id provided
		where = fmt.Sprintf(
			"%v = %v",
			hood.Dialect.Quote(hood.Dialect.Pk()),
			hood.nextMarker(),
		)
		hood.params = []interface{}{typedQuery}
	}
	if where == "" {
		panic("WHERE cannot be empty")
	}
	hood.where = fmt.Sprintf("WHERE %v", where)

	return hood
}

func (hood *Hood) Limit(limit int) *Hood {
	hood.limit = fmt.Sprintf("LIMIT %v", limit)
	return hood
}

func (hood *Hood) Offset(offset int) *Hood {
	hood.offset = fmt.Sprintf("OFFSET %v", offset)
	return hood
}

func (hood *Hood) OrderBy(key string) *Hood {
	hood.orderBy = fmt.Sprintf("ORDER BY %v", key)
	return hood
}

func (hood *Hood) Join(op, table, condition string) *Hood {
	hood.joins = append(hood.joins, fmt.Sprintf("%v JOIN %v ON %v", op, table, condition))
	return hood
}

func (hood *Hood) GroupBy(key string) *Hood {
	hood.groupBy = fmt.Sprintf("GROUP BY %v", key)
	return hood
}

func (hood *Hood) Having(condition string) *Hood {
	hood.having = fmt.Sprintf("HAVING %v", condition)
	return hood
}

func (hood *Hood) Find(out interface{}) error {
	// TODO: implement
	return nil
}

func (hood *Hood) FindAll(out []interface{}) error {
	// TODO: implement
	return nil
}

func (hood *Hood) Exec(query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (hood *Hood) Save(model interface{}) (Id, error) {
	ids, err := hood.SaveAll([]interface{}{model})
	if err != nil {
		return -1, err
	}
	return ids[0], err
}

func (hood *Hood) SaveAll(models []interface{}) ([]Id, error) {
	ids := make([]Id, 0, len(models))
	for _, v := range models {
		var (
			id  Id
			err error
		)
		model, err := interfaceToModel(v)
		if err != nil {
			return nil, err
		}
		if model.Pk != nil {
			id, err = hood.update(model)
		} else {
			id, err = hood.insert(model)
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (hood *Hood) Destroy(model interface{}) (Id, error) {
	ids, err := hood.DestroyAll([]interface{}{model})
	if err != nil {
		return -1, err
	}
	return ids[0], err
}

func (hood *Hood) DestroyAll(model []interface{}) ([]Id, error) {
	// TODO: implement
	return nil, nil
}

// Private /////////////////////////////////////////////////////////////////////

func (hood *Hood) insert(model *Model) (Id, error) {
	return 0, nil
}

func (hood *Hood) insertSql(model *Model) string {
	defer hood.reset()
	keys, _, markers := hood.sortedKeysValuesAndMarkersForModel(model, true)
	stmt := fmt.Sprintf(
		"INSERT INTO %v (%v) VALUES (%v)",
		hood.Dialect.Quote(model.Table),
		hood.Dialect.Quote(strings.Join(keys, hood.Dialect.Quote(", "))),
		strings.Join(markers, ", "),
	)
	return stmt
}

func (hood *Hood) update(model *Model) (Id, error) {
	return 0, nil
}

func (hood *Hood) updateSql(model *Model) string {
	defer hood.reset()
	keys, _, markers := hood.sortedKeysValuesAndMarkersForModel(model, true)
	stmt := fmt.Sprintf(
		"UPDATE %v (%v) VALUES (%v) WHERE %v = %v",
		hood.Dialect.Quote(model.Table),
		hood.Dialect.Quote(strings.Join(keys, hood.Dialect.Quote(", "))),
		strings.Join(markers, ", "),
		hood.Dialect.Quote(model.Pk.Name),
		hood.nextMarker(),
	)
	return stmt
}

func (hood *Hood) deleteSql(model *Model) string {
	defer hood.reset()
	stmt := fmt.Sprintf(
		"DELETE FROM %v WHERE %v = %v",
		hood.Dialect.Quote(model.Table),
		hood.Dialect.Quote(model.Pk.Name),
		hood.nextMarker(),
	)
	return stmt
}

func (hood *Hood) querySql() string {
	query := make([]string, 0, 20)
	if hood.selector != "" {
		query = append(query, hood.selector)
	}
	for _, join := range hood.joins {
		query = append(query, join)
	}
	if x := hood.where; x != "" {
		query = append(query, x)
	}
	if x := hood.groupBy; x != "" {
		query = append(query, x)
	}
	if x := hood.having; x != "" {
		query = append(query, x)
	}
	if x := hood.orderBy; x != "" {
		query = append(query, x)
	}
	if x := hood.limit; x != "" {
		query = append(query, x)
	}
	if x := hood.offset; x != "" {
		query = append(query, x)
	}
	return strings.Join(query, " ")
}

func (hood *Hood) reset() {
	hood.selector = ""
	hood.where = ""
	hood.params = []interface{}{}
	hood.paramNum = 0
	hood.limit = ""
	hood.offset = ""
	hood.orderBy = ""
	hood.joins = []string{}
	hood.groupBy = ""
	hood.having = ""
}

func (hood *Hood) sortedKeysValuesAndMarkersForModel(model *Model, excludePrimary bool) ([]string, []interface{}, []string) {
	max := len(model.Fields)
	keys := make([]string, 0, max)
	values := make([]interface{}, 0, max)
	markers := make([]string, 0, max)
	for k, _ := range model.Fields {
		if !(excludePrimary && k == model.Pk.Name) {
			keys = append(keys, k)
			markers = append(markers, hood.nextMarker())
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		values = append(values, model.Fields[k])
	}
	return keys, values, markers
}

func (hood *Hood) substituteMarkers(query string) string {
	// in order to use a uniform marker syntax, substitute
	// all question marks with the dialect marker
	chunks := []string{}
	for _, v := range strings.Split(query, "?") {
		hood.paramNum++
		chunks = append(chunks, v, hood.nextMarker())
	}
	return strings.Join(chunks, "")
}

func (hood *Hood) nextMarker() string {
	marker := hood.Dialect.Marker(hood.paramNum)
	hood.paramNum++
	return marker
}