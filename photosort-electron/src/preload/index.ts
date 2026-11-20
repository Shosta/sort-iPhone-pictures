import { contextBridge, ipcRenderer, webUtils } from 'electron'

const api = {
  send: (request: Record<string, unknown>): void => ipcRenderer.send('sidecar:send', request),
  onEvent: (callback: (event: any) => void): (() => void) => {
    const listener = (_: unknown, event: any): void => callback(event)
    ipcRenderer.on('sidecar:event', listener)
    return () => ipcRenderer.removeListener('sidecar:event', listener)
  },
  pickFolders: (title: string): Promise<string[]> => ipcRenderer.invoke('dialog:pickFolder', title),
  pickFile: (title: string): Promise<string> => ipcRenderer.invoke('dialog:pickFile', title),
  secrets: {
    available: (): Promise<boolean> => ipcRenderer.invoke('secret:available'),
    store: (account: string, secret: string): Promise<boolean> => ipcRenderer.invoke('secret:store', account, secret),
    load: (account: string): Promise<string> => ipcRenderer.invoke('secret:load', account),
    forget: (account: string): Promise<void> => ipcRenderer.invoke('secret:forget', account)
  },
  pathsFromDrop: (files: File[]): string[] => files.map((file) => webUtils.getPathForFile(file)).filter(Boolean),
  setMenuLabels: (labels: { preferences: string; tags: string; scrapbook: string }): void =>
    ipcRenderer.send('menu:labels', labels),
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
  onScrapbook: (callback: () => void): (() => void) => {
    const listener = (): void => callback()
    ipcRenderer.on('menu:scrapbook', listener)
    return () => ipcRenderer.removeListener('menu:scrapbook', listener)
  },
  getAssets: (): Promise<any> => ipcRenderer.invoke('assets:get'),
  retryAssets: (): Promise<void> => ipcRenderer.invoke('assets:retry'),
  onAssets: (callback: (state: any) => void): (() => void) => {
    const listener = (_: unknown, state: any): void => callback(state)
    ipcRenderer.on('assets:state', listener)
    return () => ipcRenderer.removeListener('assets:state', listener)
  },
  downloadsDir: (): Promise<string> => ipcRenderer.invoke('app:downloadsDir'),
  openPath: (path: string): Promise<string> => ipcRenderer.invoke('shell:reveal', path)
}

contextBridge.exposeInMainWorld('photosort', api)
export type PhotosortApi = typeof api
