import { useCallback, useRef, type ReactNode } from 'react'
import HCaptcha from '@hcaptcha/react-hcaptcha'
import { useTheme } from 'next-themes'

const SITE_KEY = import.meta.env.VITE_HCAPTCHA_SITE_KEY

/**
 * Invisible hCaptcha for Supabase auth calls. Render `captcha` once and await `getCaptchaToken()`
 * before signing in; without VITE_HCAPTCHA_SITE_KEY (local dev) it renders nothing and returns undefined.
 */
export function useCaptcha(): { captcha: ReactNode; getCaptchaToken: () => Promise<string | undefined> } {
  const ref = useRef<HCaptcha>(null)
  const { resolvedTheme } = useTheme()

  const getCaptchaToken = useCallback(async () => {
    if (!SITE_KEY) return undefined
    if (!ref.current) throw new Error('The captcha is still loading. Try again in a moment.')
    // Tokens are single-use, so each auth call gets a fresh one.
    ref.current.resetCaptcha()
    try {
      const { response } = await ref.current.execute({ async: true })
      return response
    } catch {
      throw new Error('The captcha check failed or was closed. Please try again.')
    }
  }, [])

  const captcha = SITE_KEY ? (
    <HCaptcha ref={ref} sitekey={SITE_KEY} size="invisible" theme={resolvedTheme === 'dark' ? 'dark' : 'light'} />
  ) : null

  return { captcha, getCaptchaToken }
}
