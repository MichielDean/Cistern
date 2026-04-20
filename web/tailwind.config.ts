import type { Config } from 'tailwindcss';

export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        cistern: {
          bg: '#0d1117',
          fg: '#e6edf3',
          accent: '#58a6ff',
          green: '#3fb950',
          red: '#ff7b72',
          yellow: '#d29922',
          purple: '#bc8cff',
          muted: '#8b949e',
          surface: '#161b22',
          border: '#30363d',
        },
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Consolas', 'monospace'],
      },
    },
  },
  plugins: [],
} satisfies Config;