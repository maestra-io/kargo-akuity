import { createContext } from 'react';

export type ThemePreference = 'light' | 'dark' | 'system';

export type ResolvedTheme = 'light' | 'dark';

export interface ThemeContextValue {
  preference: ThemePreference;
  resolved: ResolvedTheme;
  setPreference: (next: ThemePreference) => void;
}

export const ThemeContext = createContext<ThemeContextValue>({
  preference: 'system',
  resolved: 'light',
  setPreference: () => {}
});
