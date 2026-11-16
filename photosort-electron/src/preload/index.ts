import { contextBridge, ipcRenderer, webUtils } from 'electron'

const api = {
  send: (request: Record<string, unknown>): void => ipcRenderer.send('sidecar:send', request),
  onEvent: (callback: (event: any) => void): (() => void) => {
    const listener = (_: unknown, event: any): void => callback(event)
    ipcRenderer.on('sidecar:event', listener)
    return () => ipcRenderer.removeListener('sidecar:event', listener)
  },
  pickFolders: (title: string): Promise<string[]> => ipcRenderer.invoke('dialog:pickFolder', title),
  pathsFromDrop: (files: File[]): string[] => files.map((file) => webUtils.getPathForFile(file)).filter(Boolean),
  setMenuLabels: (labels: { preferences: string; tags: string }): void => ipcRenderer.send('menu:labels', labels),
  onPreferences: (callback: () => void): (() => void) => {
    const listener = (): void => callback()
    ipcRenderer.on('menu:preferences', listener)
    return () => ipcRenderer.removeListener('menu:preferences', listener)
  },
  onTags: (callback: () => void): (() => void) => {
    const listener = (): void => callback()
    ipcRenderer.on('menu:tags', listener)
    return () => ipcRenderer.removeListener('menu:tags', listener)
  },
  openPath: (path: string): Promise<string> => ipcRenderer.invoke('shell:reveal', path)
}

contextBridge.exposeInMainWorld('photosort', api)
export type PhotosortApi = typeof api
