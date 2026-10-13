package main

import (
	"fmt"
	"io"
	"strings"
)

// writeCompletion uses the same registry as --help so new flags are included
// automatically. Generating a script does not run dskDitto during completion.
func writeCompletion(w io.Writer, shell string, flags []flagMeta) error {
	var script string
	switch shell {
	case "bash":
		script = bashCompletion(flags)
	case "zsh":
		script = zshCompletion(flags)
	default:
		return fmt.Errorf("unsupported completion shell %q (choose bash or zsh)", shell)
	}
	if _, err := io.WriteString(w, script); err != nil {
		return fmt.Errorf("write %s completion: %w", shell, err)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func completionChoices(m flagMeta) string {
	switch m.long {
	case "completion":
		return "bash zsh"
	case "hash":
		return "sha256 blake3"
	}
	if m.argName == "" {
		return "true false"
	}
	return ""
}

// Bash accepts Go's single-dash long forms when identifying value arguments,
// while suggesting the documented --long and -short spellings.
func bashFlagPattern(m flagMeta) string {
	names := []string{"--" + m.long, "-" + m.long}
	if m.short != "" {
		names = append(names, "-"+m.short)
	}
	return strings.Join(names, "|")
}

func bashCompletion(flags []flagMeta) string {
	var valueFlags, cases, options strings.Builder
	options.WriteString("--help -h")
	for _, m := range flags {
		fmt.Fprintf(&options, " --%s", m.long)
		if m.short != "" {
			fmt.Fprintf(&options, " -%s", m.short)
		}
		if m.argName != "" {
			if valueFlags.Len() > 0 {
				valueFlags.WriteByte('|')
			}
			valueFlags.WriteString(bashFlagPattern(m))
		}
		switch {
		case completionChoices(m) != "":
			fmt.Fprintf(&cases, "        %s) choices=%s ;;\n", bashFlagPattern(m), shellQuote(completionChoices(m)))
		case m.argName == "file" || m.argName == "path":
			fmt.Fprintf(&cases, "        %s) files=1 ;;\n", bashFlagPattern(m))
		default:
			fmt.Fprintf(&cases, "        %s) return 0 ;;\n", bashFlagPattern(m))
		}
	}
	return fmt.Sprintf(`# Bash completion for dskDitto (compatible with Bash 3.2+).
# Load with: source <(dskDitto --completion bash)
_dskDitto_complete() {
    local cur="${COMP_WORDS[COMP_CWORD]}" word option='' expect='' prefix=''
    local choices='' item files=0 positional=0 i
    COMPREPLY=()

    # Go's flag parser stops at -- or the first positional path. Skip values
    # while finding that boundary, including Readline's separate '=' words.
    for ((i=1; i<COMP_CWORD; i++)); do
        word="${COMP_WORDS[i]}"
        if (( positional )); then
            continue
        fi
        if [[ -n "$expect" ]]; then
            [[ "$word" == '=' ]] && continue
            expect=''
            continue
        fi
        case "$word" in
            --|-) positional=1 ;;
            -*)
                case "${word%%%%=*}" in
                    %s)
                        [[ "$word" == *=* ]] || expect="$word"
                        ;;
                    *)
                        [[ "${COMP_WORDS[i+1]}" == '=' ]] && expect="$word"
                        ;;
                esac
                ;;
            *) positional=1 ;;
        esac
    done

    if (( positional )); then
        files=1
    else
        if [[ -n "$expect" ]]; then
            option="$expect"
            [[ "$cur" == '=' ]] && cur=''
        elif [[ "$cur" == -*=* ]]; then
            option="${cur%%%%=*}"
            prefix="$option="
            cur="${cur#*=}"
        fi
        case "$option" in
%s        '')
            if [[ "$cur" == -* ]]; then
                choices=%s
            else
                files=1
            fi
            ;;
            *) return 0 ;;
        esac
    fi

    if (( files )); then
        while IFS= read -r item; do
            COMPREPLY+=("$prefix$item")
        done < <(compgen -f -- "$cur")
    else
        while IFS= read -r item; do
            COMPREPLY+=("$prefix$item")
        done < <(compgen -W "$choices" -- "$cur")
    fi
    return 0
}
complete -o filenames -F _dskDitto_complete dskDitto
`, valueFlags.String(), cases.String(), shellQuote(options.String()))
}

func zshCompletion(flags []flagMeta) string {
	var script strings.Builder
	script.WriteString(`#compdef dskDitto
# Load after compinit with: source <(dskDitto --completion zsh)
_dskDitto() {
    _arguments -S -A '-?*' \
        '--help[Show help and exit]' \
        '-h[Show help and exit]' \
`)
	escape := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", ":", "\\:")
	for _, m := range flags {
		names := []string{"--" + m.long, "-" + m.long}
		if m.short != "" {
			names = append(names, "-"+m.short)
		}
		for _, name := range names {
			spec := "*" + name
			if m.argName == "" {
				// Boolean values are optional and accepted only with '='.
				spec += "=-[" + escape.Replace(m.usage) + "]::boolean:(true false)"
			} else {
				action := ""
				if choices := completionChoices(m); choices != "" {
					action = "(" + choices + ")"
				} else if m.argName == "file" || m.argName == "path" {
					action = "_files"
				}
				spec += "=[" + escape.Replace(m.usage) + "]:" + escape.Replace(m.argName) + ":" + action
			}
			fmt.Fprintf(&script, "        %s \\\n", shellQuote(spec))
		}
	}
	script.WriteString(`        '*:path:_files'
}

# A file named _dskDitto in fpath is autoloaded by compinit; sourcing the
# script directly registers it for the current shell instead.
if [[ "$ZSH_EVAL_CONTEXT" == *:file ]]; then
    compdef _dskDitto dskDitto
else
    _dskDitto "$@"
fi
`)
	return script.String()
}
