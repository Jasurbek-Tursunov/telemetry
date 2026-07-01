package tracing

import "go.opentelemetry.io/otel/attribute"

const (
	AttrDBSystem    = attribute.Key("db.system")
	AttrDBOperation = attribute.Key("db.operation")
	AttrDBTable     = attribute.Key("db.sql.table")
)

func DBAttrs(operation, table string) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrDBSystem.String("postgresql"),
		AttrDBOperation.String(operation),
		AttrDBTable.String(table),
	}
}
