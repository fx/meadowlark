# Changelog

## [0.1.1](https://github.com/fx/meadowlark/compare/v0.1.0...v0.1.1) (2026-04-19)


### Features

* add Air-based live reload dev tooling ([#28](https://github.com/fx/meadowlark/issues/28)) ([c33e4b2](https://github.com/fx/meadowlark/commit/c33e4b265739861148a91cd904f1c07af591e34f))
* add default voice support and fix TTS error handling ([#31](https://github.com/fx/meadowlark/issues/31)) ([5c7ae97](https://github.com/fx/meadowlark/commit/5c7ae9787eb3a3b3189901fe64a24b7c9644a038))
* add Go embed, Dockerfile, and CI workflows ([89d7acd](https://github.com/fx/meadowlark/commit/89d7acde8b48eaeeb5009e21a666e8fc43746876))
* **api:** add integration tests ([#17](https://github.com/fx/meadowlark/issues/17)) ([3d08603](https://github.com/fx/meadowlark/commit/3d0860391b2d0a99df23d6487ce639f266315682))
* **api:** HTTP server foundation with chi router and middleware ([#12](https://github.com/fx/meadowlark/issues/12)) ([1a55518](https://github.com/fx/meadowlark/commit/1a55518a8726b87fe3e6e55ae0ad9098ed8cb4d2))
* **api:** implement endpoints CRUD handlers ([#16](https://github.com/fx/meadowlark/issues/16)) ([dd3e3df](https://github.com/fx/meadowlark/commit/dd3e3dfd0172c35a48459d559f20451e7df96687))
* **api:** implement system status and voices handlers ([#13](https://github.com/fx/meadowlark/issues/13)) ([2cf7354](https://github.com/fx/meadowlark/commit/2cf7354fb3f162800a66448f8d7d87c0f50108ee))
* **api:** implement voice aliases CRUD handlers ([#15](https://github.com/fx/meadowlark/issues/15)) ([acac3ab](https://github.com/fx/meadowlark/commit/acac3ab6efa60eff701459536d7c125a3681694d))
* auto-discover models and voices from TTS endpoints ([#29](https://github.com/fx/meadowlark/issues/29)) ([0d26fb4](https://github.com/fx/meadowlark/commit/0d26fb4d9590077b8a515c04c5aec6763c954cf5))
* **frontend:** add streaming toggle and sample rate to endpoint form ([#37](https://github.com/fx/meadowlark/issues/37)) ([027e89f](https://github.com/fx/meadowlark/commit/027e89fdbb9f0cfee08d5c5fe02a78df1c34df8d))
* Go scaffold with CLI skeleton, build tooling, and docs ([5d22f36](https://github.com/fx/meadowlark/commit/5d22f36f0a0f272485f9ad0a918f0b4bc8c2a10e))
* implement domain models and SQLite store ([#5](https://github.com/fx/meadowlark/issues/5)) ([83d628d](https://github.com/fx/meadowlark/commit/83d628d6856e563ea4233db32ec802baf169965b))
* scaffold Preact frontend with Vite and Tailwind CSS v4 ([41a6afb](https://github.com/fx/meadowlark/commit/41a6afbb5bd2ecfa8181fc870675dc8862a23fce))
* **tts:** add SynthesizeStream for streaming PCM synthesis ([#36](https://github.com/fx/meadowlark/issues/36)) ([cdc72a2](https://github.com/fx/meadowlark/commit/cdc72a21613002c141b555d5d33029b129ab9f43))
* **tts:** implement synthesis proxy orchestration ([#9](https://github.com/fx/meadowlark/issues/9)) ([e023fa5](https://github.com/fx/meadowlark/commit/e023fa5890c8dbe858b6e593277751931c7f0391))
* **tts:** implement TTS HTTP client and WAV header parser ([#8](https://github.com/fx/meadowlark/issues/8)) ([de640a2](https://github.com/fx/meadowlark/commit/de640a201de8d8620fe8a4df963ff526879298f4))
* **voice:** implement voice resolver and custom input parser ([#7](https://github.com/fx/meadowlark/issues/7)) ([748b721](https://github.com/fx/meadowlark/commit/748b721420f29b4341f999f2d16dbce819ac76d6))
* **web:** add app layout, navigation, and shared CRUD components ([#20](https://github.com/fx/meadowlark/issues/20)) ([2f03a41](https://github.com/fx/meadowlark/commit/2f03a41869b64592ea6d41119bb8e64212cccd51))
* **web:** add shadcn/ui component library and theme system ([#19](https://github.com/fx/meadowlark/issues/19)) ([ceaa970](https://github.com/fx/meadowlark/commit/ceaa970ca6c25ed472b15b61af71e6e8938b2ef7))
* **web:** implement Aliases CRUD page ([#23](https://github.com/fx/meadowlark/issues/23)) ([a04cfaf](https://github.com/fx/meadowlark/commit/a04cfaf7064d4b0f17d9c46fee2ae8d27b345231))
* **web:** implement data fetching hooks and typed API client ([#18](https://github.com/fx/meadowlark/issues/18)) ([5d54356](https://github.com/fx/meadowlark/commit/5d543560f045d1ad00c0e72f7c92225ca9cb077c))
* **web:** implement Endpoints page with CRUD ([#24](https://github.com/fx/meadowlark/issues/24)) ([74549f8](https://github.com/fx/meadowlark/commit/74549f83bf81aa148f40b78d1d6a5f1ee1c91777))
* **web:** implement Settings page with server status cards ([#22](https://github.com/fx/meadowlark/issues/22)) ([eb40d28](https://github.com/fx/meadowlark/commit/eb40d28f6f0aec4b93fb8d1f6256a17328f07eed))
* **web:** implement Voices page ([#21](https://github.com/fx/meadowlark/issues/21)) ([f5eb829](https://github.com/fx/meadowlark/commit/f5eb82924bdd7f63897f6219f89290dd78c7012c))
* wire main.go + PostgreSQL Store backend ([#11](https://github.com/fx/meadowlark/issues/11)) ([cfc2b77](https://github.com/fx/meadowlark/commit/cfc2b77b639e233e0a8e5eeb1600219afa6a2d43))
* **wyoming:** implement protocol event reader/writer and TTS event types ([#6](https://github.com/fx/meadowlark/issues/6)) ([b900594](https://github.com/fx/meadowlark/commit/b900594d2f1d7476a7630661e1bafff86b57357f))
* **wyoming:** TCP server, Zeroconf, and Info builder ([#10](https://github.com/fx/meadowlark/issues/10)) ([cf4fe6f](https://github.com/fx/meadowlark/commit/cf4fe6f530c45675b74a1ad2be588925f996c812))


### Bug Fixes

* **api:** add WriteTimeout and IdleTimeout to HTTP server ([#14](https://github.com/fx/meadowlark/issues/14)) ([bd66bf8](https://github.com/fx/meadowlark/commit/bd66bf8bd72a7519e1c9dd87aec6a33d41ca593f))
* **tts:** support generic voices array format in ListVoices ([#34](https://github.com/fx/meadowlark/issues/34)) ([08e817d](https://github.com/fx/meadowlark/commit/08e817dcc46079f3d0013942252231bb490c391c))
* **voice:** resolve literal "default" voice to configured default ([#33](https://github.com/fx/meadowlark/issues/33)) ([40359bf](https://github.com/fx/meadowlark/commit/40359bf4b5ae82fcec4ed498f5af393342217ec9))
* **web:** align UI styling with reference design system ([#27](https://github.com/fx/meadowlark/issues/27)) ([35f6c24](https://github.com/fx/meadowlark/commit/35f6c2454b7024364f410b4063bdcc7d73133134))
* **web:** fix dark mode theme switcher ([#26](https://github.com/fx/meadowlark/issues/26)) ([c597d08](https://github.com/fx/meadowlark/commit/c597d0803e3a4fe1a37f01fceadabcaed996b0e9))
* **wyoming:** fix Home Assistant compatibility and add voice discovery ([#30](https://github.com/fx/meadowlark/issues/30)) ([b7164e6](https://github.com/fx/meadowlark/commit/b7164e67eaef7f331baf7113b4f04dee3d92177c))

## Changelog
