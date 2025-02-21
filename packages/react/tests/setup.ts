import "@testing-library/jest-dom";

// Suppress React 18 console errors about useEffect
const originalError = console.error;
console.error = (...args) => {
  if (typeof args[0] === "string" && args[0].includes("useEffect") && args[0].includes("strict mode")) {
    return;
  }
  originalError.call(console, ...args);
};
