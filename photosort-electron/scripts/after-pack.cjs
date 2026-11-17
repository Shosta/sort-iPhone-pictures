// Linux : enveloppe l'exécutable Electron dans un script de lancement.
// Dans une AppImage, `chrome-sandbox` ne peut pas être setuid root : Chromium
// s'arrête alors par un SIGTRAP au démarrage (« The SUID sandbox helper binary
// was found, but is not configured correctly »).
// Le script n'ajoute --no-sandbox que si le sandbox setuid n'est pas utilisable
// (donc jamais pour le .deb, qui installe chrome-sandbox en setuid root).
const fs = require('node:fs')
const path = require('node:path')

exports.default = async function afterPack(context) {
  if (context.electronPlatformName !== 'linux') return

  const exeName = context.packager.executableName
  const exePath = path.join(context.appOutDir, exeName)

  fs.renameSync(exePath, `${exePath}.bin`)
  fs.writeFileSync(
    exePath,
    `#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
SANDBOX="$HERE/chrome-sandbox"
if [ -u "$SANDBOX" ] && [ "$(stat -c %u "$SANDBOX" 2>/dev/null)" = "0" ]; then
  exec "$HERE/${exeName}.bin" "$@"
fi
exec "$HERE/${exeName}.bin" --no-sandbox "$@"
`,
    { mode: 0o755 }
  )
}
