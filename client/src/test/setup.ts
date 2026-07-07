import '@testing-library/jest-dom'

window.HTMLElement.prototype.scrollIntoView = () => {}

// jsdom does not implement matchMedia; provide a minimal stub so App initialises cleanly.
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})
