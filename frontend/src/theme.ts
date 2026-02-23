export function applyTheme(theme: string) {
  if (typeof document === 'undefined') {
    return
  }
  const dark = theme === 'sombre'
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
}
