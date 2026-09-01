import React from 'react'
import {createRoot} from 'react-dom/client'
import '@fontsource/space-mono/400.css'
import '@fontsource/space-mono/700.css'
import '@fontsource/inter/400.css'
import '@fontsource/inter/500.css'
import '@fontsource/inter/600.css'
import '@fontsource/inter/700.css'
import '@fontsource/silkscreen/400.css'
import '@fontsource/silkscreen/700.css'
import './lib/animations' // side-effect: apply animation data-attribute before render
import './main.css'
import App from './App'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)