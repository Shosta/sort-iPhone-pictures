export function shortSha(raw: unknown): string
export function resolveCommit(options?: {
  env?: Record<string, string | undefined>
  cwd?: string
  git?: (cwd: string) => string
}): string
