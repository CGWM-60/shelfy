import { act, fireEvent, render, screen } from '@testing-library/react'
import { MiniPlayer } from './MiniPlayer'
import { clearMiniPlayerState, setMiniPlayerState } from './miniPlayerState'

beforeEach(() => {
  clearMiniPlayerState()
})

test("n'affiche rien sans état actif", () => {
  render(<MiniPlayer />)
  expect(screen.queryByTestId('mini-player')).not.toBeInTheDocument()
})

test('affiche puis ferme le mini-player', async () => {
  render(<MiniPlayer />)
  await act(async () => {
    setMiniPlayerState({ mediaId: 'm1', title: 'Film A' })
  })

  expect(await screen.findByTestId('mini-player')).toBeInTheDocument()
  expect(screen.getByText('Film A')).toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: 'Fermer mini-player' }))
  expect(screen.queryByTestId('mini-player')).not.toBeInTheDocument()
})
