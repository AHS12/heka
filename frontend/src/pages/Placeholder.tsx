// Placeholder (SPEC-12 scope) — every route renders this until its page
// ships (Tasks/Schedules/Jobs/Runs/Logs in SPEC-13/14, Dashboard/Settings
// with them or SPEC-16).
export function Placeholder({title}: {title: string}) {
  return (
    <section data-testid="placeholder-page">
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-1 text-sm text-foreground/55">
        This page lands in SPEC-13/14.
      </p>
    </section>
  )
}