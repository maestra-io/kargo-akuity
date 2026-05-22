import { faDesktop, faMoon, faSun } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { Button, Tooltip } from 'antd';

import { ThemePreference } from './theme-context';
import { useTheme } from './use-theme';

const cycle: Record<ThemePreference, ThemePreference> = {
  light: 'dark',
  dark: 'system',
  system: 'light'
};

const labelFor = (preference: ThemePreference): string => {
  switch (preference) {
    case 'light':
      return 'Light theme (click to switch to dark)';
    case 'dark':
      return 'Dark theme (click to switch to system)';
    case 'system':
      return 'System theme (click to switch to light)';
  }
};

const iconFor = (preference: ThemePreference) => {
  switch (preference) {
    case 'light':
      return faSun;
    case 'dark':
      return faMoon;
    case 'system':
      return faDesktop;
  }
};

export const ThemeToggle = ({ className }: { className?: string }) => {
  const { preference, setPreference } = useTheme();

  return (
    <Tooltip title={labelFor(preference)} placement='right'>
      <Button
        className={className}
        onClick={() => setPreference(cycle[preference])}
        type='text'
        aria-label='Toggle theme'
        icon={<FontAwesomeIcon icon={iconFor(preference)} />}
      />
    </Tooltip>
  );
};
