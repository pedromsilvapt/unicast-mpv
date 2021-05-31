export class SchemaValidationError {
    expected : string[];
    received : string;
    property ?: string;

    constructor ( expected : string[] | string, received : string, property : string = null ) {
        this.property = property;
        
        if ( expected instanceof Array ) {
            this.expected = expected;
        } else {
            this.expected = [ expected ];
        }

        this.received = received;
    }

    prefix ( property : string ) : SchemaValidationError {
        if ( this.property == null ) {
            return new SchemaValidationError( this.expected, this.received, property );
        }

        return new SchemaValidationError( this.expected, this.received, property + '.' + this.property );
    }

    get message () {
        const expectations = `Expected ${ this.expected.join( ', ' ) }, got ${ this.received } instead.`;

        if ( this.property !== null ) {
            return `${ this.property }: ${ expectations }`
        } else {
            return expectations;
        }
    }


    public static toString ( errors ?: SchemaValidationResult ) {
        if ( !errors ) {
            return  null;
        }

        if ( errors instanceof Array ) {
            return errors.map( error => error.message ).join( '\n' );
        } else {
            return errors.message;
        }
    }
}

export type SchemaValidationResult = null | SchemaValidationError | SchemaValidationError[];

export abstract class TypeSchema {
    static normalize ( schema : any ) : TypeSchema {
        if ( schema instanceof TypeSchema ) {
            return schema;
        } else if ( schema instanceof Array ) {
            if ( schema.length == 0 ) {
                return ArrayTypeSchema.normalize( new AnyTypeSchema() );
            } else if ( schema.length == 1 ) {
                return ArrayTypeSchema.normalize( schema );
            } else {
                return TupleTypeSchema.normalize( schema );
            }
        } else if ( schema === String ) {
            return new StringTypeSchema();
        } else if ( schema === Number ) {
            return new NumberTypeSchema();
        } else if ( schema === Boolean ) {
            return new BooleanTypeSchema();
        } else if ( typeof schema === 'object' ) {
            return ObjectTypeSchema.normalize( schema );
        } else {
            return new ConstantTypeSchema( schema );
        }
    }

    abstract validate ( data : any ) : SchemaValidationResult;

    abstract run ( data : any ) : any;
}


export class ConstantTypeSchema extends TypeSchema {
    constant : any = null;

    constructor ( constant : any ) {
        super();

        this.constant = constant;
    }

    validate ( data : any ) : SchemaValidationResult {
        if ( data == this.constant ) {
            return null;
        }

        return new SchemaValidationError( `"${ this.constant }"`, `"${ data }"` );
    }

    run ( data : any ) {
        return this.constant;
    }
}

export class OptionalTypeSchema extends TypeSchema {
    subSchema : TypeSchema;

    defaultValue : any = null;

    constructor ( subSchema : any, defaultValue : any = null ) {
        super();

        this.subSchema = TypeSchema.normalize( subSchema );

        this.defaultValue = defaultValue;
    }

    validate ( data : any ) : SchemaValidationResult {
        if ( data === null || data === void 0 ) {
            return null;
        }

        return this.subSchema.validate( data );
    }

    run ( data : any ) {
        if ( data === null || data === void 0 ) {
            data = this.defaultValue;
        }

        return this.subSchema.run( data );
    }
}

export class AnyTypeSchema extends TypeSchema {
    validate () {
        return null;
    }

    run ( data : any ) {
        return data;
    }
}

export class UnionTypeSchema extends TypeSchema {
    typeSchemas: TypeSchema[];
    
    constructor ( ...typeSchemas: any[] ) {
        super();

        this.typeSchemas = typeSchemas.map( type => TypeSchema.normalize( type ) );
    }

    validate ( data : any ) : SchemaValidationResult {
        const errors: SchemaValidationError[] = [];

        for ( const schema of this.typeSchemas ) {
            const schemaErrors = schema.validate( data );

            if ( schemaErrors === null ) {
                return null;
            }

            if ( schemaErrors instanceof Array ) {
                errors.push( ...schemaErrors );
            } else {
                errors.push( schemaErrors );
            }
        }

        if ( errors.length === 0 ) {
            return null;
        }

        return errors;
    }

    run ( data : any ) {
        for ( const schema of this.typeSchemas ) {
            const schemaErrors = schema.validate( data );

            if ( schemaErrors === null ) {
                return schema.run( schema );
            }
        }

        return data;
    }
}

export class IntersectionTypeSchema extends TypeSchema {
    typeSchemas: TypeSchema[];
    
    constructor ( ...typeSchemas: any[] ) {
        super();

        this.typeSchemas = typeSchemas.map( type => TypeSchema.normalize( type ) );
    }

    validate ( data : any ) : SchemaValidationResult {
        const errors: SchemaValidationError[] = [];

        for ( const schema of this.typeSchemas ) {
            const schemaErrors = schema.validate( data );

            if ( schemaErrors instanceof Array ) {
                errors.push( ...schemaErrors );
            } else if ( schemaErrors != null ) {
                errors.push( schemaErrors );
            }
        }

        if ( errors.length === 0 ) {
            return null;
        }

        return errors;
    }

    run ( data : any ) {
        for ( const schema of this.typeSchemas ) {
            data = schema.run( data );
        }

        return data;
    }
}

export class StringTypeSchema extends TypeSchema {
    validate ( data : any ) : SchemaValidationResult {
        if ( typeof data === 'string' ) {
            return null;
        }

        return new SchemaValidationError( 'String', typeof data );
    }

    run ( data : any ) {
        return data;
    }
}

export class NumberTypeSchema extends TypeSchema {
    validate ( data : any ) {
        if ( typeof data === 'number' ) {
            return null;
        }

        return new SchemaValidationError( 'Number', typeof data );
    }

    run ( data : any ) {
        return data;
    }
}


export class BooleanTypeSchema extends TypeSchema {
    validate ( data : any ) {
        if ( typeof data === 'boolean' ) {
            return null;
        }

        return new SchemaValidationError( 'Boolean', typeof data );
    }

    run ( data : any ) {
        return data;
    }
}

export class TupleTypeSchema extends TypeSchema {
    static normalize ( schema : any ) : TupleTypeSchema {
        return new TupleTypeSchema( schema );
    }

    subSchema : TypeSchema[];

    constructor ( schema : any[] ) {
        super();

        this.subSchema = [];

        for ( let type of schema ) {
            this.subSchema.push( TypeSchema.normalize( type ) );
        }
    }

    validate ( data : any ) : SchemaValidationResult {
        if ( data instanceof Array ) {
            const errors = data.map( ( item, index ) => {
                    if ( this.subSchema.length <= index ) {
                        return new SchemaValidationError( 'Undefined', typeof item );
                    }

                    const errors = this.subSchema[ index ].validate( item );

                    if ( errors instanceof Array ) {
                        return errors.map( err => err.prefix( index.toString() ) );
                    } else if ( errors !== null ) {
                        return errors.prefix( index.toString() );
                    }
                } ).filter( error => error != null )
                .reduce( ( arr, errors ) => {
                    if ( errors instanceof Array ) {
                        arr.push( ...errors );
                    } else {
                        arr.push( errors )
                    }

                    return arr;
                }, [] as any[] );

                if ( errors.length === 0 ) {
                    return null;
                }

                return errors;
        }

        return new SchemaValidationError( 'Array', typeof data );
    }

    run ( data : any ) {
        if ( data instanceof Array ) {
            return data.map( ( entry, index ) => {
                if ( index < this.subSchema.length ) {
                    return this.subSchema[ index ].run( entry )
                }

                return entry;
            } );
        }
        
        return data;
    }
}

export class ArrayTypeSchema extends TypeSchema {
    static normalize ( schema : any ) : TypeSchema {
        return new ArrayTypeSchema( TypeSchema.normalize( schema[ 0 ] ) );
    }

    subSchema : TypeSchema;

    constructor ( subSchema : any ) {
        super();

        this.subSchema = TypeSchema.normalize( subSchema );
    }

    validate ( data : any ) : SchemaValidationResult {
        if ( data instanceof Array ) {
            const errors = data.map( ( item, index ) => {
                    const errors = this.subSchema.validate( item );

                    if ( errors instanceof Array ) {
                        return errors.map( err => err.prefix( index.toString() ) );
                    } else if ( errors !== null ) {
                        return errors.prefix( index.toString() );
                    }
                } )
                .filter( error => error != null )
                .reduce( ( arr, errors ) => {
                    if ( errors instanceof Array ) {
                        arr.push( ...errors );
                    } else {
                        arr.push( errors )
                    }

                    return arr;
                }, [] as any[] );

                if ( errors.length === 0 ) {
                    return null;
                }

                return errors;
        }

        return new SchemaValidationError( 'Array', typeof data );
    }

    run ( data : any ) {
        if ( data instanceof Array ) {
            return data.map( entry => this.subSchema.run( entry ) );
        }
        
        return data;
    }
}

export class ObjectTypeSchema extends TypeSchema {
    static normalize ( schema : any ) : TypeSchema {
        return new ObjectTypeSchema( schema );
    }

    subSchema : { [ key : string] : TypeSchema };

    // If true, keys that are not defined in the schema are not allowed
    strict : boolean;

    constructor ( subSchema : any, strict : boolean = false ) {
        super();

        this.subSchema = {};

        for ( let key of Object.keys( subSchema ) ) {
            this.subSchema[ key ] = TypeSchema.normalize( subSchema[ key ] );
        }

        this.strict = strict;
    }

    validate ( data : any ) : SchemaValidationResult {
        if ( data && typeof data === 'object' ) {
            const errors : SchemaValidationError[] = [];

            const requiredKeys = new Set( Object.keys( this.subSchema ) );

            for ( let key of Object.keys( data ) ) {
                if ( !( key in this.subSchema ) && this.strict ) {
                    errors.push( new SchemaValidationError( 'Undefined', typeof data[ key ], key ) )
                } else if ( key in this.subSchema ) {
                    requiredKeys.delete( key );

                    const keyErrors = this.subSchema[ key ].validate( data[ key ] );

                    if ( keyErrors instanceof Array ) {
                        errors.push( ...keyErrors.map( error => error.prefix( key ) ) );
                    } else if ( keyErrors !== null ) {
                        errors.push( keyErrors.prefix( key ) );
                    }
                }
            }

            for ( let key of requiredKeys ) {
                const keyErrors = this.subSchema[ key ].validate( void 0 );

                if ( keyErrors instanceof Array ) {
                    errors.push( ...keyErrors.map( error => error.prefix( key ) ) );
                } else if ( keyErrors !== null ) {
                    errors.push( keyErrors.prefix( key ) );
                }
            }

            if ( errors.length === 0 ) {
                return null;
            }

            return errors;
        }

        return new SchemaValidationError( 'Object', typeof data );
    }

    run ( data : any ) {
        const requiredKeys = new Set( Object.keys( this.subSchema ) );

        const result : any = {};

        for ( let key of Object.keys( data ) ) {
            if ( key in this.subSchema ) {
                requiredKeys.delete( key );

                result[ key ] = this.subSchema[ key ].run( data[ key ] );
            } else {
                result[ key ] = data[ key ];
            }
        }

        for ( let key of requiredKeys ) {
            result[ key ] = this.subSchema[ key ].run( data[ key ] );
        }

        return result;
    }
}

// Functional DSL
export function any () {
    return new AnyTypeSchema();
}

export function array ( subSchema : any ) {
    return new ArrayTypeSchema( subSchema );
}

export function object ( subSchema : any = {}, strict : boolean = false ) {
    return new ObjectTypeSchema( subSchema, strict );
}

export function union ( ...typeSchemas : any[] ) {
    return new UnionTypeSchema( ...typeSchemas );
}

export function intersection ( ...typeSchemas : any[] ) {
    return new IntersectionTypeSchema( ...typeSchemas );
}

export function tuple ( subSchema : any ) {
    return new TupleTypeSchema( subSchema );
}

export function constant ( constant : any ) {
    return new ConstantTypeSchema( constant );
}

export function number () {
    return new NumberTypeSchema();
}

export function boolean () {
    return new BooleanTypeSchema();
}

export function optional ( subSchema : any, defaultValue : any = null ) {
    return new OptionalTypeSchema( subSchema, defaultValue );
}

export function string () {
    return new StringTypeSchema();
}