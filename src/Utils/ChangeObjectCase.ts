import Case from 'case';

export type StringCase = 'upper' | 'lower' | 'capital' | 'snake' | 'pascal' | 'camel' | 'kebab' | 'header' | 'constant' | 'title' | 'sentence';

export function changeObjectCase ( obj : object, changeTo : StringCase = 'camel' ) : object {
    const changed = {};

    for ( let key of Object.keys( obj ) ) {
        changed[ Case[ changeTo ]( key ) ] = obj[ key ];
    }

    return changed;
}