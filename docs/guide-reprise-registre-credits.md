# Reprise du registre des crédits papier — guide de la bascule

Ce guide explique comment faire passer les crédits clients d'un registre papier dans FaturaCloud,
le jour de la bascule. Il s'adresse au gérant et à la personne qui saisit le registre. Le détail
technique est dans `docs/loan-register-migration-plan.md`.

## Ce que fait la reprise

- Chaque prêt du registre, **en cours ou soldé**, devient un prêt dans l'application, avec :
  - son client ;
  - ses articles ;
  - son montant d'origine ;
  - ce qui a déjà été payé.
- Les prêts **en cours** apparaissent dans le **Livre de caisse ▸ Statut des crédits**. La caisse
  les encaisse ensuite comme n'importe quel crédit, article par article.
- Les prêts **soldés** servent d'historique : on voit qu'un client a toujours payé.
- La reprise **ne touche ni au stock, ni au chiffre d'affaires, ni à la TVA, ni à la caisse du
  jour**. Ces ventes ont eu lieu avant l'application.
- Dans la liste des factures, un prêt repris porte l'étiquette **« Repris »**. Il ne peut pas être
  modifié, exporté ni supprimé ; il peut seulement être encaissé. Ce qui avait déjà été payé
  apparaît comme **« Solde d'ouverture »**.

## Avant de commencer

- **Rôle :** il faut être **administrateur** de l'organisation. Le menu **Paramètres** (la roue
  dentée) ▸ **Importer le registre des crédits** n'apparaît que pour eux.
- **Exercice comptable :** il doit exister un exercice **ouvert** qui couvre la date de bascule
  (Comptabilité ▸ Exercices comptables).
- **Fuseau horaire :** l'organisation doit être réglée sur **Africa/Tunis** (Organisations ▸
  modifier ▸ Mise en forme).
- **Stock :** le compter **le même jour**, avec le fichier de comptage de l'écran Inventaire. La reprise
  des crédits ne corrige pas le stock.

## 1. Choisir la date de bascule

Choisissez un jour précis, de préférence un jour de fermeture ou un début de mois.

- **À partir de ce jour**, tous les encaissements se font dans l'application.
- **Le registre papier s'arrête** à cette date.

## 2. Télécharger et remplir le modèle

Allez dans **Paramètres ▸ Importer le registre des crédits ▸ Télécharger le modèle**. La feuille
**« Mode d'emploi »** du fichier explique chaque colonne et montre un exemple.

**Une ligne par article.** Un prêt de trois articles prend trois lignes, avec **la même
référence**. Le client, la date et le « Déjà payé » se remplissent **sur la première ligne**
seulement.

| Colonne                | À saisir                                                                                    |
| ---------------------- | ------------------------------------------------------------------------------------------- |
| Réf. prêt \*           | La référence du prêt dans le registre, par exemple `C1-P012-3` (carnet 1, page 12, ligne 3) |
| Date de vente \*       | `jj/mm/aaaa`                                                                                |
| Client \*              | Le nom, écrit toujours de la même façon                                                     |
| CIN                    | Fortement conseillé : c'est ce qui distingue deux clients qui portent le même nom           |
| Téléphone, Téléphone 2 | Facultatifs, mais utiles                                                                    |
| Adresse, Garant        | Facultatifs                                                                                 |
| Article \*             | Le code du produit (SKU) ou son nom exact ; sinon le texte est gardé tel quel               |
| Quantité               | 1 si vide                                                                                   |
| Montant \*             | Le prix de la ligne, TTC, tel qu'écrit dans le registre : `1200` ou `1200,500`              |
| Déjà payé              | **Le total déjà payé** sur ce prêt (avance et versements). Égal au montant : prêt soldé     |
| Dernier paiement       | Facultatif : la date du dernier versement                                                   |
| Note                   | Facultatif : l'échéancier, une remarque…                                                    |

### Conseils de saisie

- **Commencez par les prêts en cours.** Ce sont eux dont la caisse a besoin le premier jour. Les
  prêts soldés peuvent suivre dans un deuxième fichier, plus tard.
- **Le travail peut se partager** entre plusieurs personnes, par carnet ou par tranche de pages.
  Chaque fichier s'importe séparément, et une référence déjà importée est simplement ignorée.
- **Écrivez le CIN et le téléphone au moins une fois par client.** Les autres prêts du même client
  s'y rattachent, même s'ils ne portent que le nom.
- **Gardez les colonnes CIN et téléphone en texte**, comme dans le modèle. Sinon Excel supprime le
  zéro du début (`01234567` deviendrait `1234567`). L'application le tolère pour le CIN, mais
  autant l'éviter.

## 3. Vérifier le fichier (import à blanc)

Sur le même écran, choisissez la **date de bascule** et le **fichier**, puis cliquez sur
**Vérifier le fichier**. **Rien n'est enregistré** à cette étape.

Le rapport affiche :

- le nombre de prêts (en cours et soldés), de clients (retrouvés et nouveaux), le total, le déjà
  payé et le reste dû ;
- les **problèmes** ligne par ligne :
  - les **erreurs** empêchent l'import ;
  - les **avertissements** sont à relire mais n'empêchent rien ;
  - les **ignorés** sont les prêts déjà importés ;
- les **totaux par client**.

### Erreurs fréquentes et solutions

| Message                                                | Que faire                                                                                                                                              |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| « N customers are named … »                            | Deux clients de l'application portent ce nom. Enregistrez le CIN ou le téléphone sur la bonne fiche (menu Clients) et dans le fichier, puis revérifiez |
| « phone … belongs to customer … »                      | Ce téléphone est celui d'un autre client (un parent, souvent). Ajoutez le CIN ou vérifiez le nom                                                       |
| « CIN … is on both … and … »                           | Le même CIN est écrit pour deux noms différents dans le fichier. Corrigez la faute de frappe                                                           |
| « … differs from row … for the same loan »             | Deux lignes du même prêt disent des choses différentes (date, client, déjà payé). Une seule valeur par prêt                                            |
| « paid to date … is more than the loan's total »       | Le déjà payé dépasse le total du prêt. Vérifiez les montants                                                                                           |
| « the sale date is after the cutover date »            | La date de vente est après la date de bascule                                                                                                          |
| « … is not a date »                                    | Écrire la date en `jj/mm/aaaa`                                                                                                                         |
| Avertissement « no product … — kept as a description » | L'article n'existe pas dans le catalogue ; il sera gardé comme texte. C'est acceptable pour un ancien prêt                                             |
| Avertissement « matched … by name only »               | Un prêt a été rattaché à un client du fichier par le seul nom. Vérifiez que c'est bien la même personne                                                |

Corrigez le fichier, puis cliquez à nouveau sur **Vérifier le fichier** jusqu'à ce qu'il n'y ait
plus d'erreur.

## 4. Comparer avec le registre

C'est l'étape la plus importante. Dans **Totaux par client**, comparez **client par client** le
total, le déjà payé et le reste dû avec le registre papier. Un écart vient presque toujours d'une
faute de frappe dans un montant ou d'un prêt oublié.

## 5. Importer

Quand la vérification ne montre plus d'erreur et que les totaux correspondent, cliquez sur
**Importer**, puis confirmez. Le bouton n'est actif qu'après une vérification réussie **du même
fichier et de la même date**.

Tout le fichier est importé d'un seul coup ; en cas de problème, rien n'est enregistré.

## 6. Contrôler après l'import

- Ouvrez le **Livre de caisse ▸ Statut des crédits**. Les prêts repris y sont, avec leur reste dû.
- Exportez le Statut des crédits en **Excel** et cochez-le une dernière fois contre le registre.
- Ouvrez un prêt depuis **Factures** (étiquette **« Repris »**) : un bandeau rappelle qu'il vient du
  registre papier.

## 7. Les encaissements pendant la saisie

Un client qui paie **entre la saisie et l'import** a deux solutions :

- **Avant l'import :** mettez à jour le « Déjà payé » du prêt dans le fichier.
- **Après l'import :** encaissez-le normalement dans le **Livre de caisse** (Enregistrer un
  paiement), à la date réelle.

## 8. Annuler un import

En bas de l'écran, la liste **Imports** propose **Annuler** pour chaque import. Cela supprime
ses prêts, ainsi que les clients qu'il a créés s'ils ne servent à rien d'autre.

**Ce n'est plus possible dès qu'un prêt de cet import a été encaissé dans l'application.** Pour
corriger un seul prêt après cela, encaissez ou régularisez-le dans le Livre de caisse.

## 9. Clôturer le registre papier

Le jour de la bascule, **inscrivez sur la dernière page du registre** :

- « Repris dans FaturaCloud le jj/mm/aaaa » ;
- le total des restes dus.

Rangez-le ensuite. Il ne doit plus servir.
