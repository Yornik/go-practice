package envfile_test

import "testing"

func TestParseComments(t *testing.T) {
	runParseCases(t, []parseCase{
		{"comment after a value", "LOG_LEVEL=info # quiet", map[string]string{"LOG_LEVEL": "info"}},
		{"tab before the comment", "A=1\t# note", map[string]string{"A": "1"}},
		{"several blanks before the comment", "A=x    # note", map[string]string{"A": "x"}},
		{"comment after a value with blanks", "MSG=two words # note", map[string]string{"MSG": "two words"}},
		{"only a comment after =", "KEY= #c", map[string]string{"KEY": ""}},
		{"only a comment after = and a tab", "KEY=\t#c", map[string]string{"KEY": ""}},
		{"comment that holds =", "A=1 # B=2", map[string]string{"A": "1"}},
		{"comment with CRLF", "A=1 # note\r\nB=2\r\n", map[string]string{"A": "1", "B": "2"}},
		// A # that does not follow a blank is data.
		{"# right after =", "COLOR=#ff0000", map[string]string{"COLOR": "#ff0000"}},
		{"# inside a word", "A=b#c", map[string]string{"A": "b#c"}},
		{"# inside a word, then a comment", "A=b#c #d", map[string]string{"A": "b#c"}},
		{"URL with a fragment", "DOCS=https://wiki.vesper-labs.example/runbooks#disk", map[string]string{"DOCS": "https://wiki.vesper-labs.example/runbooks#disk"}},
		{"## in a value", "A=x##y", map[string]string{"A": "x##y"}},
	})
}

func TestParseQuotes(t *testing.T) {
	runParseCases(t, []parseCase{
		{"double quotes", `A="hello world"`, map[string]string{"A": "hello world"}},
		{"single quotes", `A='hello world'`, map[string]string{"A": "hello world"}},
		{"single quote inside double quotes", `A="it's"`, map[string]string{"A": "it's"}},
		{"double quotes inside single quotes", `A='say "hi"'`, map[string]string{"A": `say "hi"`}},
		{"# inside quotes", `A="a # b"`, map[string]string{"A": "a # b"}},
		{"color in quotes", `COLOR="#ff0000"`, map[string]string{"COLOR": "#ff0000"}},
		{"blanks inside quotes are kept", "A=\"  x\t \"", map[string]string{"A": "  x\t "}},
		{"= inside quotes", `A="a=b"`, map[string]string{"A": "a=b"}},
		{"empty double quotes", `A=""`, map[string]string{"A": ""}},
		{"empty single quotes", `A=''`, map[string]string{"A": ""}},
		{"blanks before the opening quote", "A=  \"x\"\nB=\t'y'", map[string]string{"A": "x", "B": "y"}},
		{"blanks around = and quotes", `A = "x y"`, map[string]string{"A": "x y"}},
		{"blanks after the closing quote", "A=\"x\"   \nB='y'\t", map[string]string{"A": "x", "B": "y"}},
		{"comment after the closing quote", `A="x" # note`, map[string]string{"A": "x"}},
		{"comment right after the closing quote", `A='x'#note`, map[string]string{"A": "x"}},
		{"comment with a quote in it", `A="x" # it's "fine"`, map[string]string{"A": "x"}},
		{"quotes with CRLF", "A=\"x y\"\r\nB='z'\r\n", map[string]string{"A": "x y", "B": "z"}},
		{"backslashes are data", `A="C:\new\temp"`, map[string]string{"A": `C:\new\temp`}},
		{"backslash before the closing quote", `A='x\'`, map[string]string{"A": `x\`}},
		{"export with quotes", `export A="x y"`, map[string]string{"A": "x y"}},
		// A quote that is not the first byte of the value is data.
		{"quote inside an unquoted value", `A=x"y`, map[string]string{"A": `x"y`}},
		{"apostrophe in an unquoted value", `A=it's`, map[string]string{"A": "it's"}},
		{"quotes at the end of an unquoted value", `A=x"y"`, map[string]string{"A": `x"y"`}},
		{"last definition wins with quotes", "A='1'\nA=\"2\"", map[string]string{"A": "2"}},
	})
}

func TestParseQuotesNoExpansion(t *testing.T) {
	runParseCases(t, []parseCase{
		{"$$ unquoted", "DB_PASSWORD=pa$$w0rd", map[string]string{"DB_PASSWORD": "pa$$w0rd"}},
		{"$$ in double quotes", `DB_PASSWORD="pa$$w0rd"`, map[string]string{"DB_PASSWORD": "pa$$w0rd"}},
		{"$$ in single quotes", `DB_PASSWORD='pa$$w0rd'`, map[string]string{"DB_PASSWORD": "pa$$w0rd"}},
		{"${HOME} unquoted", "DATA_DIR=${HOME}/models", map[string]string{"DATA_DIR": "${HOME}/models"}},
		{"${HOME} in double quotes", `DATA_DIR="${HOME}/models"`, map[string]string{"DATA_DIR": "${HOME}/models"}},
		{"$VAR of a key defined earlier", "A=1\nB=$A", map[string]string{"A": "1", "B": "$A"}},
		{"command substitution", "NOW=$(date)\nOLD=`date`", map[string]string{"NOW": "$(date)", "OLD": "`date`"}},
		{"lone $", "PRICE=5$", map[string]string{"PRICE": "5$"}},
	})
}
