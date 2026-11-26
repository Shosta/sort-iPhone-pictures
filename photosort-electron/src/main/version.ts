// Version SemVer de l'application + SHA court du commit (ex. « 1.0.0 (b89b82e) »).
export function formatVersion(version: string, commit: string): string {
  return commit ? `${version} (${commit})` : version
}

export function isPrerelease(version: string): boolean {
  return version.includes('-')
}
