package logstorage

import (
	"fmt"
	"strings"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
)

// pipeRename implements '| rename ...' pipe.
//
// See https://docs.victoriametrics.com/victorialogs/logsql/#rename-pipe
type pipeRename struct {
	// srcFields contains a list of source fields to rename
	srcFields []string

	// dstFields contains a list of destination fields
	dstFields []string
}

func (pr *pipeRename) String() string {
	if len(pr.srcFields) == 0 {
		logger.Panicf("BUG: pipeRename must contain at least a single srcField")
	}

	a := make([]string, len(pr.srcFields))
	for i, srcField := range pr.srcFields {
		dstField := pr.dstFields[i]
		a[i] = quoteTokenIfNeeded(srcField) + " as " + quoteTokenIfNeeded(dstField)
	}
	return "rename " + strings.Join(a, ", ")
}

func (pr *pipeRename) updateNeededFields(neededFields, unneededFields fieldsSet) {
	m := make(map[string]int)
	for i, dstField := range pr.dstFields {
		if neededFields.contains(dstField) && !unneededFields.contains(dstField) {
			m[pr.srcFields[i]]++
		}
	}
	if neededFields.contains("*") {
		// update only unneeded fields
		unneededFields.addAll(pr.dstFields)
		for i, srcField := range pr.srcFields {
			if m[srcField] > 0 {
				unneededFields.remove(pr.srcFields[i])
			}
		}
	} else {
		// update only needed fields and reset unneeded fields
		neededFields.removeAll(pr.dstFields)
		for i, srcField := range pr.srcFields {
			if m[srcField] > 0 {
				neededFields.add(pr.srcFields[i])
			} else {
				neededFields.remove(pr.srcFields[i])
			}
		}
		unneededFields.reset()
	}
}

func (pr *pipeRename) newPipeProcessor(_ int, _ <-chan struct{}, _ func(), ppBase pipeProcessor) pipeProcessor {
	return &pipeRenameProcessor{
		pr:     pr,
		ppBase: ppBase,
	}
}

type pipeRenameProcessor struct {
	pr     *pipeRename
	ppBase pipeProcessor
}

func (prp *pipeRenameProcessor) writeBlock(workerID uint, br *blockResult) {
	if len(br.timestamps) == 0 {
		return
	}

	br.renameColumns(prp.pr.srcFields, prp.pr.dstFields)
	prp.ppBase.writeBlock(workerID, br)
}

func (prp *pipeRenameProcessor) flush() error {
	return nil
}

func parsePipeRename(lex *lexer) (*pipeRename, error) {
	if !lex.isKeyword("rename", "mv") {
		return nil, fmt.Errorf("expecting 'rename' or 'mv'; got %q", lex.token)
	}

	var srcFields []string
	var dstFields []string
	for {
		lex.nextToken()
		srcField, err := parseFieldName(lex)
		if err != nil {
			return nil, fmt.Errorf("cannot parse src field name: %w", err)
		}
		if lex.isKeyword("as") {
			lex.nextToken()
		}
		dstField, err := parseFieldName(lex)
		if err != nil {
			return nil, fmt.Errorf("cannot parse dst field name: %w", err)
		}

		srcFields = append(srcFields, srcField)
		dstFields = append(dstFields, dstField)

		switch {
		case lex.isKeyword("|", ")", ""):
			pr := &pipeRename{
				srcFields: srcFields,
				dstFields: dstFields,
			}
			return pr, nil
		case lex.isKeyword(","):
		default:
			return nil, fmt.Errorf("unexpected token: %q; expecting ',', '|' or ')'", lex.token)
		}
	}
}
