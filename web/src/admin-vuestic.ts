import type { App } from 'vue'
import { createVuestic } from 'vuestic-ui'
import 'vuestic-ui/css'
import './styles/admin.css'

export function installAdminUI(app: App) {
  app.use(createVuestic({
    config: {
      colors: {
        variables: {
          primary: '#8b6cff',
          success: '#45d6aa',
          warning: '#f0ae5b',
          danger: '#ff7185',
          info: '#5fb8ff',
          backgroundPrimary: '#151620',
          backgroundSecondary: '#0c0d13',
        },
      },
    },
  }))
}
