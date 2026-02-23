import { NavLink } from 'react-router-dom'

const links = [
  { to: '/', label: 'Téléchargements' },
  { to: '/library', label: 'Bibliothèque' },
  { to: '/ai/report', label: 'Rapport IA' },
  { to: '/debrid', label: 'Débrideurs' },
  { to: '/settings', label: 'Paramètres' }
]

export function Nav() {
  return (
    <nav className="flex flex-wrap gap-2 md:gap-3">
      {links.map((link) => (
        <NavLink
          key={link.to}
          to={link.to}
          end={link.to === '/'}
          className={({ isActive }) =>
            `rounded-xl border px-3 py-2 text-sm font-medium transition ${
              isActive
                ? 'border-brand-600 bg-brand-600 text-white shadow-sm'
                : 'border-slate-300 bg-white text-slate-700 hover:border-brand-300 hover:bg-brand-50'
            }`
          }
        >
          {link.label}
        </NavLink>
      ))}
    </nav>
  )
}
