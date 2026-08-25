// components/tasks/YamlEditor.tsx (SPEC-13 §4) — CodeMirror with the YAML
// grammar plugin. Errors render ABOVE the editor (not buried under it); the
// text is the single source on this tab — on failure it is preserved
// byte-for-byte and no save signal is sent.
import CodeMirror from '@uiw/react-codemirror'
import {yaml} from '@codemirror/lang-yaml'
import {EditorView} from '@codemirror/view'
import {useTheme} from '../../lib/theme'

export function YamlEditor({
  value,
  onChange,
  errors,
}: {
  value: string
  onChange: (text: string) => void
  errors: string[]
}) {
  const {resolved} = useTheme()
  return (
    <div className="space-y-2">
      {errors.length > 0 && (
        <ul
          role="alert"
          data-testid="yaml-errors"
          className="space-y-1 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-950/60 dark:text-red-300"
        >
          {errors.map((err) => (
            <li key={err}>• {err}</li>
          ))}
        </ul>
      )}
      <div
        data-testid="yaml-editor"
        className="heka-scroll overflow-hidden rounded-xl border border-zinc-200 bg-zinc-50/80 focus-within:border-accent focus-within:ring-1 focus-within:ring-accent-ring dark:border-zinc-700 dark:bg-zinc-900/70"
      >
        <CodeMirror
          value={value}
          onChange={onChange}
          height="480px"
          theme={resolved === 'dark' ? 'dark' : 'light'}
          extensions={[
            yaml(),
            EditorView.contentAttributes.of({'aria-label': 'Task YAML'}),
          ]}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            highlightActiveLine: true,
          }}
        />
      </div>
    </div>
  )
}