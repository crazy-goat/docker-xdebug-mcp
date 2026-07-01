FROM alpine:3.20

RUN apk add --no-cache \
    php83 \
    php83-fpm \
    php83-pdo \
    php83-pdo_mysql \
    php83-pecl-xdebug \
    composer

COPY php83-fpm.conf /etc/php83/php-fpm.d/www.conf
COPY xdebug.ini /etc/php83/conf.d/xdebug.ini
COPY xdebug-on xdebug-off xdebug-status /usr/local/bin/
RUN chmod +x /usr/local/bin/xdebug-on /usr/local/bin/xdebug-off /usr/local/bin/xdebug-status

WORKDIR /var/www/html

CMD ["php-fpm83", "-F"]
