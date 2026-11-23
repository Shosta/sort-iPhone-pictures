// Linux : enveloppe l'exécutable Electron dans un script de lancement.
// Dans une AppImage, `chrome-sandbox` ne peut pas être setuid root : Chromium
// s'arrête alors par un SIGTRAP au démarrage (« The SUID sandbox helper binary
// was found, but is not configured correctly ») si les espaces de noms
// utilisateur non privilégiés ne sont pas disponibles non plus.
//
// Le script garde donc le sandbox Chromium chaque fois qu'il est utilisable :
//   1. chrome-sandbox setuid root (cas du .deb) ;
//   2. espaces de noms utilisateur non privilégiés (cas de la plupart des distributions).
// Il ne retombe sur --no-sandbox que si aucun des deux n'est disponible, en le
// signalant sur stderr. PHOTOSORT_REQUIRE_SANDBOX=1 interdit ce repli : le
// lancement échoue au lieu de s'exécuter sans sandbox.
const fs = require('node:fs')
const path = require('node:path')

function wrapperScript(exeName, sysctlDir = '/proc/sys') {
  return `#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
BIN="$HERE/${exeName}.bin"
SANDBOX="$HERE/chrome-sandbox"
SYSCTL="${sysctlDir}"

if [ -u "$SANDBOX" ] && [ "$(stat -c %u "$SANDBOX" 2>/dev/null)" = "0" ]; then
  exec "$BIN" "$@"
fi

userns_usable() {
  [ "$(cat "$SYSCTL/kernel/unprivileged_userns_clone" 2>/dev/null)" = "0" ] && return 1
  [ "$(cat "$SYSCTL/user/max_user_namespaces" 2>/dev/null)" = "0" ] && return 1
  [ "$(cat "$SYSCTL/kernel/apparmor_restrict_unprivileged_userns" 2>/dev/null)" = "1" ] && return 1
  command -v unshare >/dev/null 2>&1 || return 1
  unshare --user true >/dev/null 2>&1
}

if userns_usable; then
  exec "$BIN" "$@"
fi

if [ "$PHOTOSORT_REQUIRE_SANDBOX" = "1" ]; then
  echo "Photosort : aucun sandbox Chromium utilisable et PHOTOSORT_REQUIRE_SANDBOX=1, lancement refuse." >&2
  echo "Installez le paquet .deb (chrome-sandbox setuid) ou activez les espaces de noms utilisateur non privilegies." >&2
  exit 1
fi

echo "Photosort : AVERTISSEMENT - aucun sandbox Chromium utilisable, lancement avec --no-sandbox." >&2
echo "Pour un lancement sandboxe, installez le paquet .deb ou activez les espaces de noms utilisateur non privilegies." >&2
exec "$BIN" --no-sandbox "$@"
`
}

exports.wrapperScript = wrapperScript

exports.default = async function afterPack(context) {
  if (context.electronPlatformName !== 'linux') return

  const exeName = context.packager.executableName
  const exePath = path.join(context.appOutDir, exeName)

  fs.renameSync(exePath, `${exePath}.bin`)
  fs.writeFileSync(exePath, wrapperScript(exeName), { mode: 0o755 })
}
