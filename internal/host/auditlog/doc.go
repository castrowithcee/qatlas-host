// Package auditlog is qhost's part of the action log: the entry type, the actions it can record, and the
// validation of both. Appending, the hash chain, the lock, retention, torn-line markers, reading and
// checking are the kit's auditlog package; this package fixes how qhost uses it.
package auditlog
