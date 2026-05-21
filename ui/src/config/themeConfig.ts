import { theme as antdTheme } from 'antd';
import { ThemeConfig } from 'antd/es/config-provider';
import { MapToken } from 'antd/es/theme/interface';

import { ResolvedTheme } from '@ui/features/common/theme/theme-context';

const sharedToken: Partial<MapToken> = {
  fontSizeHeading1: 28,
  fontSizeHeading2: 24,
  fontSizeHeading3: 20,
  fontSizeHeading4: 18,
  fontSizeHeading5: 14,
  borderRadius: 8,
  fontFamily: 'Poppins, sans-serif'
};

const lightToken: Partial<MapToken> = {
  ...sharedToken,
  colorPrimary: '#30476c',
  colorText: '#454545',
  colorBgLayout: '#f7f8fa'
};

const darkToken: Partial<MapToken> = {
  ...sharedToken,
  colorPrimary: '#6f8cc7',
  colorBgLayout: '#141618'
};

const lightComponents: ThemeConfig['components'] = {
  Menu: {
    itemActiveBg: '#ebedf1',
    itemSelectedBg: '#ebedf1',
    itemHoverBg: '#f8f9fb',
    itemHeight: 36
  },
  Layout: {
    headerBg: '#fff',
    headerHeight: 50,
    headerPadding: '0 16px'
  },
  Card: {
    borderRadius: 8
  },
  Button: {
    contentFontSizeSM: 13
  }
};

const darkComponents: ThemeConfig['components'] = {
  Menu: {
    itemHeight: 36
  },
  Layout: {
    headerHeight: 50,
    headerPadding: '0 16px'
  },
  Card: {
    borderRadius: 8
  },
  Button: {
    contentFontSizeSM: 13
  }
};

export const getThemeConfig = (mode: ResolvedTheme): ThemeConfig => ({
  algorithm: mode === 'dark' ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
  token: mode === 'dark' ? darkToken : lightToken,
  components: mode === 'dark' ? darkComponents : lightComponents
});

// Kept for backwards compatibility with any importers (e.g. extensions). Light mode default.
export const token = lightToken;
export const themeConfig = getThemeConfig('light');
