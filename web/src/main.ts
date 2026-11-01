import '@fontsource-variable/bricolage-grotesque/standard.css';
import '@fontsource-variable/jetbrains-mono';
import './app.css';
import { mount } from 'svelte';
import App from './App.svelte';
import { initTheme } from './lib/theme';

initTheme();
mount(App, { target: document.getElementById('app')! });
