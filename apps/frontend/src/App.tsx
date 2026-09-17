import './App.css'
import CameraGrid from './components/CameraGrid'

function App() {
  return (
    <div className="app">
      <header className="app__header">
        <h1>Tapo Manager</h1>
      </header>
      <main>
        <CameraGrid />
      </main>
    </div>
  )
}

export default App
