// The npm packages whose code ships in the browser bundle, their licence
// notices and an SBOM of them (ADR-044). The bundler knows which modules
// the bundle holds, so the list follows the code that actually ships, not
// the lockfile. Both outputs depend on the installed packages alone, so the
// same lockfile writes the same bytes (ADR-021).
import { Buffer } from 'node:buffer';
import { mkdir, readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

import type { Plugin } from 'vite';

/** How the plugin finds the bundled packages and where it writes. */
export interface ThirdPartyOptions {
	/** The stylesheet whose package imports Tailwind inlines into the bundle's CSS. */
	stylesheet: string;
	/** The file the notices go to, inside the bundle. */
	notices: string;
	/** The file the SBOM goes to. It stays outside the bundle, which the binary embeds. */
	sbom: string;
	/** What the SBOM describes. */
	application: { name: string; version: string };
	/** Licences of packages whose manifest declares none, as their licence file grants them. */
	undeclaredLicences: Record<string, string>;
	/** Packages that ship no licence file. Their notice falls back to the standard text. */
	withoutLicenceFile: string[];
}

/** One bundled package, as its manifest and the lockfile describe it. */
interface BundledPackage {
	name: string;
	version: string;
	licence: string;
	author: string;
	licenceTexts: string[];
	resolved: string;
	integrity: string;
}

/** An entry of the lockfile's packages map. */
interface LockedPackage {
	resolved?: string;
	integrity?: string;
}

// Virtual modules that the bundler fills with its own code, by the package
// that code comes from.
const virtualModulePackages: Record<string, string> = {
	'\0rolldown/': 'rolldown',
	'\0vite/': 'vite'
};

// Standard licence texts, for packages that ship no licence file.
const standardTexts: Record<string, string> = {
	ISC: `Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
`
};

// A licence file's name starts with one of these words, in any case.
const licenceFilePattern = /^(licen[cs]e|copying|notice)/i;

// packageNameIn returns the package name that a path below node_modules
// starts with, such as "@sveltejs/kit" for "@sveltejs/kit/src/index.js".
function packageNameIn(subpath: string): string {
	const segments = subpath.split('/');
	const isScoped = segments[0]?.startsWith('@') ?? false;
	return segments.slice(0, isScoped ? 2 : 1).join('/');
}

// packageDirOf returns the directory of the package that a module comes
// from, an empty string for the project's own modules, and undefined for a
// virtual module of unknown origin.
function packageDirOf(root: string, moduleId: string): string | undefined {
	const id = moduleId.split('?')[0] ?? moduleId;
	if (id.startsWith('\0')) {
		const known = Object.entries(virtualModulePackages).find(([prefix]) => id.startsWith(prefix));
		return known === undefined ? undefined : path.join(root, 'node_modules', known[1]);
	}
	const marker = '/node_modules/';
	const at = id.lastIndexOf(marker);
	if (at < 0) {
		return '';
	}
	const subpath = id.slice(at + marker.length);
	return id.slice(0, at + marker.length) + packageNameIn(subpath);
}

// stylesheetPackageDirs returns the packages that the stylesheet imports by
// name. Tailwind inlines them, so the bundler never sees them as modules.
async function stylesheetPackageDirs(root: string, stylesheet: string): Promise<string[]> {
	const css = await readFile(path.join(root, stylesheet), 'utf8');
	const imports = css.matchAll(/@import\s+['"]([^'"./][^'"]*)['"]/g);
	return [...imports].map((match) =>
		path.join(root, 'node_modules', packageNameIn(match[1] ?? ''))
	);
}

// authorOf words a manifest's author field, which is a string or an object.
function authorOf(author: unknown): string {
	if (typeof author === 'string') {
		return author;
	}
	if (typeof author === 'object' && author !== null && 'name' in author) {
		return String(author.name);
	}
	return '';
}

async function licenceTextsIn(dir: string): Promise<string[]> {
	const files = (await readdir(dir)).filter((file) => licenceFilePattern.test(file)).sort();
	return Promise.all(files.map((file) => readFile(path.join(dir, file), 'utf8')));
}

// licenceOf returns the licence a package is under: the one its manifest
// declares, or else the one the options name for it. Exactly one of them
// must exist, so that a stale exception stands out.
function licenceOf(name: string, declared: unknown, options: ThirdPartyOptions): string {
	const named = options.undeclaredLicences[name];
	if (typeof declared === 'string' && named !== undefined) {
		throw new Error(`${name} now declares ${declared}: remove it from undeclaredLicences`);
	}
	if (typeof declared === 'string') {
		return declared;
	}
	if (named === undefined) {
		throw new Error(
			`${name} declares no licence: add the one its licence file grants to undeclaredLicences`
		);
	}
	return named;
}

// standardTextFor returns the notice for a package that ships no licence
// file: the standard text of its licence, with its author as the holder.
function standardTextFor(pkg: Omit<BundledPackage, 'licenceTexts'>): string {
	const text = standardTexts[pkg.licence];
	if (text === undefined) {
		throw new Error(
			`${pkg.name} ships no licence file, and no standard text of ${pkg.licence} is known`
		);
	}
	return `The package ships no licence file. This is the standard text of the licence it declares.\n\nCopyright (c) ${pkg.author}\n\n${text}`;
}

// noticeTextsOf returns the licence texts of a package's notice: the
// licence files it ships, or the standard text for a listed exception.
// Exactly one of them must exist, so that a stale exception stands out.
async function noticeTextsOf(
	pkg: Omit<BundledPackage, 'licenceTexts'>,
	dir: string,
	options: ThirdPartyOptions
): Promise<string[]> {
	const texts = await licenceTextsIn(dir);
	const isException = options.withoutLicenceFile.includes(pkg.name);
	if (texts.length > 0 && isException) {
		throw new Error(`${pkg.name} now ships a licence file: remove it from withoutLicenceFile`);
	}
	if (texts.length > 0) {
		return texts;
	}
	if (!isException) {
		throw new Error(
			`${pkg.name} ships no licence file: list it in withoutLicenceFile after checking its licence`
		);
	}
	return [standardTextFor(pkg)];
}

async function readPackage(
	root: string,
	dir: string,
	lockfile: Record<string, LockedPackage>,
	options: ThirdPartyOptions
): Promise<BundledPackage> {
	const manifest = JSON.parse(await readFile(path.join(dir, 'package.json'), 'utf8')) as Record<
		string,
		unknown
	>;
	const name = String(manifest.name);
	const locked = lockfile[path.relative(root, dir)];
	if (locked?.resolved === undefined || locked.integrity === undefined) {
		throw new Error(`${name} has no resolved address or integrity in the lockfile`);
	}
	const pkg = {
		name,
		version: String(manifest.version),
		licence: licenceOf(name, manifest.license, options),
		author: authorOf(manifest.author),
		resolved: locked.resolved,
		integrity: locked.integrity
	};
	return { ...pkg, licenceTexts: await noticeTextsOf(pkg, dir, options) };
}

function noticesOf(packages: BundledPackage[]): string {
	const rule = '='.repeat(72);
	const entries = packages.map((pkg) =>
		[
			rule,
			`${pkg.name} ${pkg.version}`,
			`Licence: ${pkg.licence}`,
			'',
			...pkg.licenceTexts.map((text) => text.trimEnd() + '\n')
		].join('\n')
	);
	const preface =
		'Third-party software in the PatchCaster web interface\n\n' +
		"The web interface's browser bundle holds code from the npm packages\n" +
		'below. Each entry names a package, its licence and the licence text\n' +
		'it ships with.\n';
	return [preface, ...entries].join('\n');
}

// purlOf returns a package's package URL. A scope's "@" is encoded.
function purlOf(pkg: BundledPackage): string {
	return `pkg:npm/${pkg.name.replace(/^@/, '%40')}@${pkg.version}`;
}

// byPurl orders packages by their package URL, comparing code units, so
// that the order never depends on the locale or the ICU version.
function byPurl(a: BundledPackage, b: BundledPackage): number {
	const [left, right] = [purlOf(a), purlOf(b)];
	if (left === right) {
		return 0;
	}
	return left < right ? -1 : 1;
}

// distinct keeps one entry per package URL. A version installed in two
// places ships the same code.
function distinct(packages: BundledPackage[]): BundledPackage[] {
	return [...new Map(packages.map((pkg) => [purlOf(pkg), pkg])).values()];
}

// hashesOf turns an integrity field, such as "sha512-<base64>", into
// CycloneDX hashes.
function hashesOf(integrity: string): { alg: string; content: string }[] {
	const algorithms: Record<string, string> = {
		sha1: 'SHA-1',
		sha256: 'SHA-256',
		sha384: 'SHA-384',
		sha512: 'SHA-512'
	};
	return integrity.split(/\s+/).map((entry) => {
		const [algorithm = '', digest = ''] = entry.split(/-(.*)/s);
		const alg = algorithms[algorithm];
		if (alg === undefined) {
			throw new Error(`unknown integrity algorithm ${algorithm}`);
		}
		return { alg, content: Buffer.from(digest, 'base64').toString('hex') };
	});
}

// sbomOf writes a CycloneDX 1.6 document. It carries no timestamp and no
// serial number, both optional, so that it stays reproducible.
function sbomOf(packages: BundledPackage[], application: ThirdPartyOptions['application']): string {
	const components = packages.map((pkg) => {
		const scoped = pkg.name.startsWith('@');
		const [group, name] = scoped ? pkg.name.split('/') : [undefined, pkg.name];
		return {
			type: 'library',
			'bom-ref': purlOf(pkg),
			...(group === undefined ? {} : { group }),
			name,
			version: pkg.version,
			licenses: [{ expression: pkg.licence }],
			purl: purlOf(pkg),
			hashes: hashesOf(pkg.integrity),
			externalReferences: [{ type: 'distribution', url: pkg.resolved }]
		};
	});
	const document = {
		bomFormat: 'CycloneDX',
		specVersion: '1.6',
		version: 1,
		metadata: {
			component: { type: 'application', 'bom-ref': application.name, ...application }
		},
		components
	};
	return JSON.stringify(document, null, 2) + '\n';
}

/**
 * thirdParty lists the npm packages whose code the browser bundle holds.
 * It writes their licence notices into the bundle and an SBOM of them
 * next to it. The build fails on a package it cannot describe.
 */
export function thirdParty(options: ThirdPartyOptions): Plugin {
	return {
		name: 'patchcaster:third-party',
		apply: 'build',
		// Only the browser build ships. The server build exists for the
		// framework's analysis alone.
		applyToEnvironment: (environment) => environment.name === 'client',
		async generateBundle(_outputOptions, bundle) {
			const root = this.environment.config.root;
			const dirs = new Set(await stylesheetPackageDirs(root, options.stylesheet));
			for (const output of Object.values(bundle)) {
				if (output.type !== 'chunk') {
					continue;
				}
				for (const [id, module] of Object.entries(output.modules)) {
					if (module.renderedLength === 0) {
						continue;
					}
					const dir = packageDirOf(root, id);
					if (dir === undefined) {
						this.error(`the virtual module ${JSON.stringify(id)} ships code of no known package`);
					}
					if (dir !== '') {
						dirs.add(dir);
					}
				}
			}

			const lock = JSON.parse(await readFile(path.join(root, 'package-lock.json'), 'utf8')) as {
				packages: Record<string, LockedPackage>;
			};
			const read = await Promise.all(
				[...dirs].map((dir) => readPackage(root, dir, lock.packages, options))
			);
			const packages = distinct(read).sort(byPurl);

			this.emitFile({ type: 'asset', fileName: options.notices, source: noticesOf(packages) });
			const sbom = path.join(root, options.sbom);
			await mkdir(path.dirname(sbom), { recursive: true });
			await writeFile(sbom, sbomOf(packages, options.application));
		}
	};
}
